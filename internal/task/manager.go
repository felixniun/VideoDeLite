// Package task implements the local task queue and encoding lifecycle.
//
// Frozen rules honored here:
//   - §35: exactly one encoding task runs at a time in V1
//   - §34: at most one automatic hardware → CPU fallback, encoder errors only
//   - §38: no checkpoint resume after restart
//   - §52–§54: output validation is mandatory, exit code 0 alone is not success
//   - §55: authorization changes never kill a running FFmpeg (red line)
package task

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"videodelite/internal/auth"
	"videodelite/internal/encoder"
	"videodelite/internal/errs"
	"videodelite/internal/ffmpegx"
	"videodelite/internal/history"
	"videodelite/internal/logging"
	"videodelite/internal/media"
	"videodelite/internal/settings"
	"videodelite/internal/validation"
)

type Manager struct {
	mu    sync.Mutex
	order []string
	tasks map[string]*Task

	Log      *logging.Logger
	Settings *settings.Store
	History  *history.Store
	Auth     *auth.Service

	FFmpegBin  string
	FFprobeBin string
	Detector   *encoder.Detector

	emit   func(name string, data any)
	notify  chan struct{}
	stop    chan struct{}

	// concurrency: V1 plan §35 freezes the default at 1 (sequential);
	// the owner-approved option raises it up to maxConcurrency in
	// Professional mode. The semaphore is rebuilt when idle.
	concurrency int
	slots       chan struct{}
}

const maxConcurrency = 3

func NewManager(log *logging.Logger, st *settings.Store,
	hist *history.Store, au *auth.Service, ffmpegBin, ffprobeBin string,
	det *encoder.Detector) *Manager {
	return &Manager{
		tasks:       map[string]*Task{},
		Log:         log,
		Settings:    st,
		History:     hist,
		Auth:        au,
		FFmpegBin:   ffmpegBin,
		FFprobeBin:  ffprobeBin,
		Detector:    det,
		notify:      make(chan struct{}, 1),
		stop:        make(chan struct{}),
		concurrency: 1,
		slots:       make(chan struct{}, 1),
	}
}

// SetConcurrency adjusts how many encodes run simultaneously (1..3).
// Applies immediately when the queue is idle; otherwise on the next batch.
func (m *Manager) SetConcurrency(n int) {
	if n < 1 {
		n = 1
	}
	if n > maxConcurrency {
		n = maxConcurrency
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.concurrency == n {
		return
	}
	m.concurrency = n
	if !m.hasActiveLocked() {
		m.slots = make(chan struct{}, n)
	}
	m.Log.Info(fmt.Sprintf("task concurrency set to %d", n))
}

func (m *Manager) Concurrency() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.concurrency
}

// hasActiveLocked reports whether any task is handed to a worker.
func (m *Manager) hasActiveLocked() bool {
	for _, t := range m.tasks {
		if !t.State.Terminal() {
			return true
		}
	}
	return false
}

// SetEmitter wires Wails event emission (nil in CLI mode).
func (m *Manager) SetEmitter(fn func(name string, data any)) {
	m.emit = fn
}

func (m *Manager) fire(name string, data any) {
	if m.emit != nil {
		m.emit(name, data)
	}
}

// Start launches the single-slot dispatcher and sweeps orphaned temp dirs
// left by a previous abnormal exit (plan §38: no task recovery after
// restart, but task-owned temp must be cleaned).
func (m *Manager) Start() {
	m.sweepOrphanTemps()
	go m.loop()
}

// sweepOrphanTemps removes .videodelite-temp subtrees in known output
// locations. Safe at startup because V1 runs a single instance and no task
// can be encoding yet.
func (m *Manager) sweepOrphanTemps() {
	roots := []string{}
	if st := m.Settings.Get(); st.OutputDir != "" {
		roots = append(roots, st.OutputDir)
	}
	// Also sweep output dirs from recent history (users may have changed it).
	if entries, err := m.History.List(50); err == nil {
		seen := map[string]bool{}
		for _, e := range entries {
			if dir := filepath.Dir(e.OutputPath); dir != "" && !seen[dir] {
				seen[dir] = true
				roots = append(roots, dir)
			}
		}
	}
	for _, root := range roots {
		tmp := filepath.Join(root, ".videodelite-temp")
		entries, err := os.ReadDir(tmp)
		if err != nil {
			continue
		}
		removed := 0
		for _, e := range entries {
			if e.IsDir() {
				if err := os.RemoveAll(filepath.Join(tmp, e.Name())); err == nil {
					removed++
				}
			}
		}
		// Remove the now-empty parent too (ignore errors).
		os.Remove(tmp)
		if removed > 0 {
			m.Log.Info(fmt.Sprintf("swept %d orphaned temp dir(s) under %s", removed, tmp))
		}
	}
}

func (m *Manager) Stop() {
	close(m.stop)
}

func (m *Manager) loop() {
	for {
		select {
		case <-m.stop:
			return
		case <-m.notify:
		}
		// Hand every waiting task to a worker goroutine; workers block on
		// the semaphore until a concurrency slot frees up.
		for {
			t := m.nextWaiting()
			if t == nil {
				break
			}
			m.mu.Lock()
			slots := m.slots
			m.mu.Unlock()
			go func(t *Task, slots chan struct{}) {
				slots <- struct{}{} // acquire slot (blocks while busy)
				m.runTask(t)
				<-slots // release slot
				m.kick()
			}(t, slots)
		}
	}
}

func (m *Manager) kick() {
	select {
	case m.notify <- struct{}{}:
	default:
	}
}

func (m *Manager) nextWaiting() *Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range m.order {
		t := m.tasks[id]
		if t.State == StateWaiting && !t.dispatched {
			t.dispatched = true
			return t
		}
	}
	return nil
}

// Enqueue validates and queues a new task.
func (m *Manager) Enqueue(req CreateRequest) (string, error) {
	if req.Mode == "" {
		req.Mode = "simple"
	}
	if req.Mode == "professional" && !m.Auth.CanUseProfessional() {
		return "", fmt.Errorf("Professional 功能需要有效授权（授权状态机 Rule 02）")
	}
	st, err := os.Stat(req.InputPath)
	if err != nil || st.IsDir() {
		return "", fmt.Errorf("输入文件不可用: %s", req.InputPath)
	}

	t := &Task{
		ID:        newID(),
		State:     StateWaiting,
		Request:   req,
		CreatedAt: time.Now(),
	}
	m.mu.Lock()
	m.tasks[t.ID] = t
	m.order = append(m.order, t.ID)
	m.mu.Unlock()

	m.Log.Info(fmt.Sprintf("task %s queued: %s (%s)", t.ID, req.InputPath, req.Mode))
	m.fire("task:update", t.view())
	m.kick()
	return t.ID, nil
}

// Tasks returns a snapshot ordered by creation.
func (m *Manager) Tasks() []*TaskView {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*TaskView, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.tasks[id].view())
	}
	return out
}

func (m *Manager) get(id string) *Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tasks[id]
}

func (m *Manager) setState(t *Task, s State) {
	m.mu.Lock()
	t.State = s
	if s == StatePreparing && t.StartedAt == (time.Time{}) {
		t.StartedAt = time.Now()
	}
	if s.Terminal() {
		t.EndedAt = time.Now()
	}
	v := t.view()
	m.mu.Unlock()
	m.fire("task:update", v)
}

// Pause suspends the FFmpeg process (process-level, not a checkpoint).
func (m *Manager) Pause(id string) error {
	m.mu.Lock()
	t := m.tasks[id]
	if t == nil {
		m.mu.Unlock()
		return fmt.Errorf("task not found: %s", id)
	}
	if t.State == StateEncoding && t.run != nil {
		if err := t.run.Suspend(); err != nil {
			m.mu.Unlock()
			return err
		}
		t.State = StatePaused
		v := t.view()
		m.mu.Unlock()
		m.fire("task:update", v)
		return nil
	}
	if t.State == StateWaiting || t.State == StatePreparing {
		t.pausePending = true
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()
	return fmt.Errorf("task is not pausable in state %s", t.State)
}

// Resume un-suspends the FFmpeg process.
func (m *Manager) Resume(id string) error {
	m.mu.Lock()
	t := m.tasks[id]
	if t == nil {
		m.mu.Unlock()
		return fmt.Errorf("task not found: %s", id)
	}
	if t.State == StatePaused && t.run != nil {
		if err := t.run.Resume(); err != nil {
			m.mu.Unlock()
			return err
		}
		t.State = StateEncoding
		v := t.view()
		m.mu.Unlock()
		m.fire("task:update", v)
		return nil
	}
	if t.pausePending {
		t.pausePending = false
	}
	m.mu.Unlock()
	return fmt.Errorf("task is not resumable in state %s", t.State)
}

// Cancel stops a waiting/running task. Already-running work is terminated;
// the temp output is cleaned up (§38).
func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	t := m.tasks[id]
	if t == nil {
		m.mu.Unlock()
		return fmt.Errorf("task not found: %s", id)
	}
	if t.State.Terminal() {
		m.mu.Unlock()
		return fmt.Errorf("task already finished: %s", t.State)
	}
	t.cancelReq = true
	run := t.run
	m.mu.Unlock()
	if run != nil {
		// Resume first so a suspended process can be reaped cleanly.
		_ = run.Resume()
		return run.Kill()
	}
	// Waiting/Preparing: terminal now.
	m.finishCanceled(t)
	return nil
}

// Remove drops a finished task from the list.
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.tasks[id]
	if t == nil {
		return fmt.Errorf("task not found: %s", id)
	}
	if !t.State.Terminal() {
		return fmt.Errorf("task still active: %s", t.State)
	}
	delete(m.tasks, id)
	for i, x := range m.order {
		if x == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	m.fire("task:removed", map[string]string{"id": id})
	return nil
}

// Retry re-queues a finished task as a fresh entry.
func (m *Manager) Retry(id string) (string, error) {
	t := m.get(id)
	if t == nil {
		return "", fmt.Errorf("task not found: %s", id)
	}
	if !t.State.Terminal() {
		return "", fmt.Errorf("task still active: %s", t.State)
	}
	return m.Enqueue(t.Request)
}

func (m *Manager) finishCanceled(t *Task) {
	m.cleanupTempDir(t.TempDir)
	m.setState(t, StateCanceled)
	m.recordHistory(t, "Canceled", "")
}

func (m *Manager) fail(t *Task, code, message string) {
	m.cleanupTempDir(t.TempDir)
	m.mu.Lock()
	t.ErrCode = code
	t.ErrMessage = message
	m.mu.Unlock()
	m.setState(t, StateFailed)
	m.recordHistory(t, "Failed", code+": "+message)
	m.Log.Error(fmt.Sprintf("task %s failed: %s: %s", t.ID, code, message))
}

// cleanupTempDir removes a task's temporary directory and then removes the
// shared parent when no other task is using it. os.Remove on the parent is
// safe under parallel encoding: it only succeeds if the directory is empty.
func (m *Manager) cleanupTempDir(tempDir string) {
	if tempDir == "" {
		return
	}
	if err := os.RemoveAll(tempDir); err != nil {
		m.Log.Warn(fmt.Sprintf("could not remove task temp dir %s: %v", tempDir, err))
		return
	}
	tempRoot := filepath.Dir(tempDir)
	if filepath.Base(tempRoot) != ".videodelite-temp" {
		return
	}
	if err := os.Remove(tempRoot); err != nil && !os.IsNotExist(err) {
		// The parent may still contain another active task's temp directory.
		// Keep it in that case; a later task cleanup will remove it when empty.
		if entries, readErr := os.ReadDir(tempRoot); readErr == nil && len(entries) == 0 {
			m.Log.Warn(fmt.Sprintf("could not remove empty temp root %s: %v", tempRoot, err))
		}
	}
}

// ---- the encoding lifecycle ----

func (m *Manager) runTask(t *Task) {
	// A cancel may have landed while the task was Waiting.
	m.mu.Lock()
	canceled := t.cancelReq
	m.mu.Unlock()
	if canceled {
		m.finishCanceled(t)
		return
	}

	m.setState(t, StatePreparing)

	analyzer := media.NewAnalyzer(m.FFprobeBin)
	mi, err := analyzer.Analyze(t.Request.InputPath)
	if err != nil {
		m.fail(t, errs.InputFileError, err.Error())
		return
	}
	m.mu.Lock()
	t.InputInfo = mi
	m.mu.Unlock()

	st := m.Settings.Get()

	// Build the encoding config.
	var cfg encoder.EncodingConfig
	if t.Request.Mode == "professional" {
		cfg = t.Request.Config
		if cfg.Container == "" {
			cfg.Container = "mp4"
		}
		if cfg.VideoCodec == "" {
			cfg.VideoCodec = "h264"
		}
	} else {
		cfg = encoder.BuildSimpleConfig(mi, t.Request.Codec, t.Request.Quality, t.Request.Container)
	}
	outDir := t.Request.OutputDir
	if outDir == "" {
		outDir = st.OutputDir
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		m.fail(t, errs.OutputFileError, "无法创建输出目录: "+err.Error())
		return
	}
	cfg.Encoder = t.Request.Encoder
	if cfg.Encoder == "" {
		cfg.Encoder = "auto"
	}

	// Disk space preflight (§93: warn, don't block).
	if free, ok := diskFree(outDir); ok && free < uint64(mi.FileSize) {
		m.warn(t, "目标磁盘可用空间可能不足")
	}

	// Encoder selection (§33: hardware first when enabled).
	allowHW := st.HardwareEncode || t.Request.Mode == "professional"
	sel, err := m.Detector.Select(cfg.VideoCodec, cfg.Encoder, allowHW)
	if err != nil {
		if ue, ok := err.(*encoder.EncoderUnavailableError); ok {
			m.fail(t, errs.EncoderUnavailable, ue.Reason)
		} else {
			m.fail(t, errs.UnknownError, err.Error())
		}
		return
	}
	m.mu.Lock()
	t.EncoderLabel = sel.Label
	t.EncoderArg = sel.Arg
	t.EncoderFamily = sel.Family
	t.Config = cfg
	m.mu.Unlock()

	// Output naming (§57): name_low/mid/high.ext, professional name_compressed.
	ext := ".mp4"
	if cfg.Container == "mkv" {
		ext = ".mkv"
	}
	suffix := "_compressed"
	if t.Request.Mode == "simple" {
		suffix = "_" + orDefault(t.Request.Quality, "mid")
	}
	base := strings.TrimSuffix(mi.FileName, filepath.Ext(mi.FileName))
	outName := sanitizeName(base) + suffix + ext

	// Temp dir lives next to the output so the final move is an atomic
	// same-volume rename (§92: per-task temp, cleaned on failure).
	tempDir := filepath.Join(outDir, ".videodelite-temp", t.ID)
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		m.cleanupTempDir(tempDir)
		m.fail(t, errs.OutputFileError, "无法创建临时目录: "+err.Error())
		return
	}
	m.mu.Lock()
	t.TempDir = tempDir
	m.mu.Unlock()
	tempOut := filepath.Join(tempDir, outName)

	tlog := logging.OpenTaskLog(m.Log.Dir(), t.ID)
	defer tlog.Close()

	for attempt := 0; ; attempt++ {
		build, err := encoder.BuildCommand(t.Request.InputPath, tempOut, mi, cfg, sel)
		if err != nil {
			m.fail(t, errs.FFmpegArgumentError, err.Error())
			return
		}
		for _, w := range build.Warnings {
			m.warn(t, w)
		}
		m.mu.Lock()
		t.EncoderLabel = sel.Label
		t.EncoderArg = sel.Arg
		t.EncoderFamily = sel.Family
		t.Config = cfg
		m.mu.Unlock()

		tlog.WriteLine(fmt.Sprintf("== VideoDelite task %s ==", t.ID))
		tlog.WriteLine(fmt.Sprintf("input:  %s", t.Request.InputPath))
		tlog.WriteLine(fmt.Sprintf("output: %s", tempOut))
		tlog.WriteLine(fmt.Sprintf("ffmpeg %s", strings.Join(build.Args, " ")))

		m.setState(t, StateEncoding)
		m.mu.Lock()
		started := t.StartedAt == (time.Time{})
		if started {
			t.StartedAt = time.Now()
		}
		m.mu.Unlock()

		run, startErr := ffmpegx.Start(m.FFmpegBin, build.Args,
			func(p ffmpegx.Progress) { m.onProgress(t, mi.Duration, p) },
			func(line string) { tlog.WriteLine(line) })
		if startErr != nil {
			m.fail(t, errs.UnknownError, "无法启动 FFmpeg: "+startErr.Error())
			return
		}
		m.mu.Lock()
		t.run = run
		pending := t.pausePending
		m.mu.Unlock()
		t.EncodedOnce = true
		if pending {
			_ = run.Suspend()
			m.setState(t, StatePaused)
		}

		waitErr := run.Wait()

		m.mu.Lock()
		t.run = nil
		cancelReq := t.cancelReq
		m.mu.Unlock()

		if cancelReq {
			m.finishCanceled(t)
			return
		}
		if waitErr == nil {
			break // success → validate
		}

		info := errs.Classify(run.ErrTail(), waitErr.Error())
		// §34: single CPU fallback, encoder-related failures only.
		if info.EncoderRelated() && encoder.IsHardware(sel.Family) && !t.fallbackUsed &&
			m.Detector != nil {
			t.fallbackUsed = true
			m.warn(t, "硬件编码失败（"+info.Code+"），已自动回退 CPU 软件编码")
			m.Log.Warn(fmt.Sprintf("task %s: hw encoder failed (%s), falling back to CPU", t.ID, info.Code))
			cpuSel, err := m.Detector.Select(cfg.VideoCodec, "cpu", true)
			if err == nil {
				sel = cpuSel
				continue // one retry with CPU
			}
		}
		m.fail(t, info.Code, info.Message)
		return
	}

	// ---- validation (§52) ----
	m.setState(t, StateValidating)
	valAnalyzer := media.NewAnalyzer(m.FFprobeBin)
	rep, err := validation.Validate(valAnalyzer, tempOut, mi, cfg, sel.Arg)
	if err != nil {
		m.fail(t, errs.ValidationFailed, err.Error())
		return
	}
	for _, w := range rep.Warnings {
		m.warn(t, w)
	}
	if !rep.Pass {
		var sb strings.Builder
		for _, c := range rep.Checks {
			if !c.Pass && !c.Warning {
				fmt.Fprintf(&sb, "%s: expected %s, got %s; ", c.Name, c.Expected, c.Actual)
			}
		}
		m.fail(t, errs.ValidationFailed, sb.String())
		return
	}

	// ---- placement with conflict strategy (§55 plan) ----
	placed, note := placeFile(tempOut, filepath.Join(outDir, outName), st.ConflictStrategy)
	if note != "" {
		m.warn(t, note)
	}
	m.cleanupTempDir(tempDir)
	m.mu.Lock()
	t.OutputPath = placed
	t.Percent = 1
	m.mu.Unlock()

	m.setState(t, StateCompleted)
	m.recordHistory(t, "Completed", "")
	m.Log.Info(fmt.Sprintf("task %s completed: %s", t.ID, placed))
	m.kick() // start the next queued task promptly
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func sanitizeName(name string) string {
	// Keep the visible name as-is; only strip characters illegal on Windows.
	repl := strings.Map(func(r rune) rune {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		return r
	}, name)
	return strings.TrimSpace(repl)
}

// placeFile moves tempOut into the destination honoring the conflict
// strategy. Returns the final path and an optional user-facing note.
func placeFile(tempOut, dst, strategy string) (string, string) {
	_, err := os.Stat(dst)
	if err == nil {
		switch strategy {
		case "overwrite":
			os.Remove(dst)
		case "skip":
			os.Remove(tempOut)
			return dst, "输出文件已存在，按策略跳过写入"
		case "cancel":
			os.Remove(tempOut)
			return dst, "输出文件已存在，已取消写入"
		default: // auto: name_1, name_2, …
			ext := filepath.Ext(dst)
			stem := strings.TrimSuffix(dst, ext)
			for i := 1; ; i++ {
				cand := fmt.Sprintf("%s_%d%s", stem, i, ext)
				if _, err := os.Stat(cand); err != nil {
					dst = cand
					break
				}
			}
		}
	}
	if err := os.Rename(tempOut, dst); err != nil {
		// Cross-volume safety net (should not trigger: temp is same-volume).
		if moveErr := moveCrossVolume(tempOut, dst); moveErr != nil {
			return dst, "移动输出文件失败: " + err.Error()
		}
	}
	return dst, ""
}

func moveCrossVolume(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	buf := make([]byte, 1024*1024)
	for {
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				break
			}
			return rerr
		}
	}
	return os.Remove(src)
}

func (m *Manager) warn(t *Task, msg string) {
	m.mu.Lock()
	t.Warnings = append(t.Warnings, msg)
	m.mu.Unlock()
}

func (m *Manager) onProgress(t *Task, duration float64, p ffmpegx.Progress) {
	m.mu.Lock()
	t.FPS = p.FPS
	t.Speed = p.Speed
	t.OutSize = p.TotalSize
	t.BitrateKbps = p.BitrateKbps
	if duration > 0 {
		t.Percent = p.OutTimeSec / duration
		if t.Percent > 0.999 {
			t.Percent = 0.999
		}
		if p.Speed > 0.01 {
			t.ETASec = (duration - p.OutTimeSec) / p.Speed
		}
	} else {
		t.Percent = 0
	}
	now := time.Now()
	if now.Sub(t.lastEmit) < 300*time.Millisecond {
		m.mu.Unlock()
		return
	}
	t.lastEmit = now
	v := t.view()
	m.mu.Unlock()
	m.fire("task:progress", v)
}

func (m *Manager) recordHistory(t *Task, status, errSummary string) {
	e := history.Entry{
		CreatedAt:     time.Now().Format("2006-01-02 15:04:05"),
		InputName:     baseNameOf(t.Request.InputPath),
		InputPath:     t.Request.InputPath,
		OutputName:    baseNameOf(t.OutputPath),
		OutputPath:    t.OutputPath,
		Container:     t.Config.Container,
		VideoCodec:    t.Config.VideoCodec,
		Encoder:       t.EncoderLabel,
		Quality:       orDefault(t.Request.Quality, t.Request.Mode),
		BitrateKbps:   int64(t.Config.BitrateMbps * 1000),
		Status:        status,
		ErrorSummary:  errSummary,
		Warnings:      strings.Join(t.Warnings, "; "),
	}
	if t.InputInfo != nil {
		e.InputSize = t.InputInfo.FileSize
		e.DurationSec = t.InputInfo.Duration
	}
	if st, err := os.Stat(t.OutputPath); err == nil && status == "Completed" {
		e.OutputSize = st.Size()
		if e.InputSize > 0 {
			e.Ratio = float64(st.Size()) / float64(e.InputSize)
		}
	}
	if t.EndedAt != (time.Time{}) && t.StartedAt != (time.Time{}) {
		e.EncodeTimeSec = t.EndedAt.Sub(t.StartedAt).Seconds()
	}
	if err := m.History.Add(e); err != nil {
		m.Log.Error("history write failed: " + err.Error())
	}
}
