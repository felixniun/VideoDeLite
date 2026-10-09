package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"videodelite/internal/account"
	"videodelite/internal/auth"
	"videodelite/internal/db"
	"videodelite/internal/encoder"
	"videodelite/internal/ffmpegx"
	"videodelite/internal/history"
	"videodelite/internal/logging"
	"videodelite/internal/media"
	"videodelite/internal/paths"
	"videodelite/internal/settings"
	"videodelite/internal/task"
)

const appVersion = "1.0.0-dev"

// App is the Wails binding surface. It is a thin facade: all real logic lives
// in internal/* packages so the encoding domain never depends on the
// account domain (boundary spec §27).
type App struct {
	ctx     context.Context
	version string

	p        *paths.Paths
	log      *logging.Logger
	db       *db.DB
	sett     *settings.Store
	hist     *history.Store
	auth     *auth.Service
	acct     *account.Service
	tasks    *task.Manager
	det      *encoder.Detector
	analyzer *media.Analyzer

	ffmpegBin  string
	ffprobeBin string
}

func NewApp() (*App, error) {
	p, err := paths.Ensure()
	if err != nil {
		return nil, fmt.Errorf("user data dirs: %w", err)
	}
	logg := logging.New(p.Logs)
	d, err := db.Open(p.DB)
	if err != nil {
		return nil, fmt.Errorf("local database: %w", err)
	}
	sett := settings.NewStore(d.SQL())
	hist := history.NewStore(d.SQL())
	au := auth.New()

	s := sett.Get()
	ffmpegBin, err := ffmpegx.FindTool("ffmpeg", s.FFmpegPath)
	if err != nil {
		logg.Warn("ffmpeg not found yet: " + err.Error())
	}
	ffprobeBin, err := ffmpegx.FindTool("ffprobe", s.FFprobePath)
	if err != nil {
		logg.Warn("ffprobe not found yet: " + err.Error())
	}
	det := encoder.NewDetector(ffmpegBin)

	a := &App{
		version:    appVersion,
		p:          p,
		log:        logg,
		db:         d,
		sett:       sett,
		hist:       hist,
		auth:       au,
		det:        det,
		analyzer:   media.NewAnalyzer(ffprobeBin),
		ffmpegBin:  ffmpegBin,
		ffprobeBin: ffprobeBin,
	}
	a.tasks = task.NewManager(logg, sett, hist, au, ffmpegBin, ffprobeBin, det)
	a.tasks.SetConcurrency(s.ParallelTasks) // restore saved scheduling (1 = frozen simple default)
	a.tasks.Start()
	// Account subsystem (Phase 10): server URL from settings, env override
	// for development, builtin default otherwise.
	acct := account.NewService(au, d.SQL(),
		firstNonEmpty(s.AccountServer, os.Getenv("VIDEODELITE_API")))
	account.SetAppVersion(appVersion)
	a.acct = acct
	// Development-only unlock for the Professional gate (MVP has no account
	// server yet). Never set in production installs; the state machine and
	// gate stay exactly as specified.
	if os.Getenv("VIDEODELITE_DEV_PRO") == "1" {
		au.SetState(auth.StateAuthorized, "dev-local")
		logg.Warn("Professional gate unlocked via VIDEODELITE_DEV_PRO (development only)")
	}
	a.acct.Startup()
	logg.Info(fmt.Sprintf("VideoDelite %s started (ffmpeg=%s)", appVersion, ffmpegBin))
	return a, nil
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.tasks.SetEmitter(func(name string, data any) {
		runtime.EventsEmit(ctx, name, data)
	})
}

func (a *App) shutdown(ctx context.Context) {
	a.log.Info("VideoDelite shutting down")
}

// ---------- info / environment ----------

type AppInfoVO struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	FFmpegPath  string `json:"ffmpegPath"`
	FFprobePath string `json:"ffprobePath"`
	FFmpegOK    bool   `json:"ffmpegOk"`
	Platform    string `json:"platform"`
}

func (a *App) AppInfo() AppInfoVO {
	_, ffErr := ffmpegx.FindTool("ffmpeg", a.sett.Get().FFmpegPath)
	return AppInfoVO{
		Name:        "VideoDelite",
		Version:     a.version,
		FFmpegPath:  a.ffmpegBin,
		FFprobePath: a.ffprobeBin,
		FFmpegOK:    ffErr == nil,
		Platform:    "windows",
	}
}

// SystemLanguage follows the Windows UI language (plan §9: first run maps
// zh-CN/zh-Hans → 简体中文, everything else → English).
func (a *App) SystemLanguage() string { return systemLanguage() }

// SystemTheme reads the Windows personalization setting (plan §10).
func (a *App) SystemTheme() string { return systemTheme() }

// ---------- settings ----------

func (a *App) GetSettings() settings.Settings { return a.sett.Get() }

func (a *App) SaveSettings(s settings.Settings) error {
	if s.OutputDir == "" {
		s.OutputDir = paths.DefaultOutputDir()
	}
	if err := a.sett.Save(s); err != nil {
		return err
	}
	// Account server changes take effect immediately.
	if firstNonEmpty(s.AccountServer, os.Getenv("VIDEODELITE_API")) != "" {
		a.acct.SetBaseURL(firstNonEmpty(s.AccountServer, os.Getenv("VIDEODELITE_API")))
	}
	a.log.Info("settings saved")
	return nil
}

// ---------- import / analyze ----------

func (a *App) ScanPaths(inputs []string) ([]string, error) {
	return media.ExpandPaths(inputs)
}

func (a *App) AnalyzeFiles(pathsList []string) ([]*media.MediaInfo, error) {
	if len(pathsList) == 0 {
		return []*media.MediaInfo{}, nil
	}
	files, err := media.ExpandPaths(pathsList)
	if err != nil {
		return nil, err
	}
	out := make([]*media.MediaInfo, 0, len(files))
	for _, f := range files {
		mi, err := a.analyzer.Analyze(f)
		if err != nil {
			return nil, err
		}
		out = append(out, mi)
	}
	return out, nil
}

// EstimateSimpleBitrate exposes the frozen Simple Mode table to the UI so the
// config panel can preview the target bitrate (single source of truth).
func (a *App) EstimateSimpleBitrate(width, height int, fps float64, codec, quality string) float64 {
	return encoder.SimpleBitrateMbps(width, height, fps, codec, quality)
}

func (a *App) GetEncoderOptions(codec string) []encoder.EncoderOption {
	return a.det.OptionsFor(codec)
}

// ---------- tasks ----------

func (a *App) CreateSimpleTasks(files []string, codec, quality, container, outputDir, encoderSel string) ([]string, error) {
	expanded, err := media.ExpandPaths(files)
	if err != nil {
		return nil, err
	}
	if len(expanded) == 0 {
		return nil, fmt.Errorf("没有可导入的视频文件")
	}
	ids := make([]string, 0, len(expanded))
	for _, f := range expanded {
		id, err := a.tasks.Enqueue(task.CreateRequest{
			InputPath: f,
			Mode:      "simple",
			Codec:     codec,
			Quality:   quality,
			Container: container,
			OutputDir: outputDir,
			Encoder:   encoderSel,
		})
		if err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (a *App) CreateProfessionalTask(req task.CreateRequest) (string, error) {
	req.Mode = "professional"
	return a.tasks.Enqueue(req)
}

func (a *App) GetTasks() []*task.TaskView { return a.tasks.Tasks() }

// SetTaskConcurrency selects sequential (1) or parallel (up to 3) encoding.
// Default stays 1 per plan §35; the professional-mode option is an
// owner-approved extension.
func (a *App) SetTaskConcurrency(n int) {
	if n < 1 {
		n = 1
	}
	a.tasks.SetConcurrency(n)
	s := a.sett.Get()
	s.ParallelTasks = n
	_ = a.sett.Save(s)
}

func (a *App) GetTaskConcurrency() int { return a.tasks.Concurrency() }

func (a *App) PauseTask(id string) error  { return a.tasks.Pause(id) }
func (a *App) ResumeTask(id string) error { return a.tasks.Resume(id) }
func (a *App) CancelTask(id string) error { return a.tasks.Cancel(id) }
func (a *App) RemoveTask(id string) error { return a.tasks.Remove(id) }
func (a *App) RetryTask(id string) (string, error) { return a.tasks.Retry(id) }

// ---------- history ----------

func (a *App) GetHistory() ([]history.Entry, error) { return a.hist.List(500) }

func (a *App) ClearHistory() error { return a.hist.Clear() }

// ---------- dialogs / shell ----------

func (a *App) PickFolder() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择输出文件夹",
	})
}

func (a *App) PickVideoFiles() ([]string, error) {
	return runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择视频文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "视频文件 (*.mp4;*.mkv;*.mov;*.avi;*.webm;…)", Pattern: "*.mp4;*.mkv;*.mov;*.avi;*.webm;*.flv;*.ts;*.m2ts;*.wmv;*.m4v;*.mpg;*.mpeg;*.vob;*.3gp;*.ogv;*.rm;*.rmvb"},
			{DisplayName: "所有文件 (*.*)", Pattern: "*.*"},
		},
	})
}

func (a *App) RevealPath(path string) error {
	fullPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("无法解析路径 %q: %w", path, err)
	}
	st, err := os.Stat(fullPath)
	if err != nil {
		return fmt.Errorf("找不到输出文件或目录 %q: %w", fullPath, err)
	}
	folder := fullPath
	if !st.IsDir() {
		folder = filepath.Dir(fullPath)
	}
	return openFolder(folder)
}

// OpenLogFolder opens %LOCALAPPDATA%/VideoDelite/Logs in Explorer.
func (a *App) OpenLogFolder() error {
	return openFolder(a.p.Logs)
}

// ---------- auth / account ----------

func (a *App) GetAuthState() auth.Snapshot { return a.auth.Snapshot() }

func (a *App) AccountRegister(username, email, password, inviteCode string) (string, error) {
	return a.acct.Register(username, email, password, inviteCode)
}

func (a *App) AccountVerifyEmail(email, code string) error {
	if err := a.acct.VerifyEmail(email, code); err != nil {
		return err
	}
	a.log.Info("email verified: " + maskUser(email))
	return nil
}

func (a *App) AccountLogin(email, password string) (auth.Snapshot, error) {
	snap, err := a.acct.Login(email, password)
	if err != nil {
		return snap, err
	}
	a.log.Info("account login: " + maskUser(email))
	return snap, nil
}

func (a *App) AccountLogout() auth.Snapshot { return a.acct.Logout() }

func (a *App) AccountDelete(password string) error {
	err := a.acct.DeleteAccount(password)
	if err == nil {
		a.log.Info("account deleted")
	}
	return err
}

func (a *App) AccountSync() auth.Snapshot {
	a.acct.Sync()
	return a.auth.Snapshot()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ---------- logs ----------

func (a *App) LogsUsage() logging.Usage { return logging.UsageOf(a.p.Logs) }

// ExportLogs writes a local ZIP of the logs directory. Nothing is uploaded
// (plan §44); usernames in paths are masked before archiving.
func (a *App) ExportLogs() (string, error) {
	tmp, err := os.MkdirTemp("", "videodelite-logs-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	zipPath := filepath.Join(tmp, fmt.Sprintf("videodelite-logs-%s.zip", time.Now().Format("20060102-150405")))
	if err := zipDir(a.p.Logs, zipPath); err != nil {
		return "", err
	}
	dst, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "导出日志",
		DefaultFilename: filepath.Base(zipPath),
		Filters:         []runtime.FileFilter{{DisplayName: "ZIP 压缩包 (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil {
		return "", err
	}
	if dst == "" {
		return "", nil // user canceled
	}
	if err := copyFile(zipPath, dst); err != nil {
		return "", err
	}
	a.log.Info("logs exported to " + maskUser(dst))
	return dst, nil
}

func zipDir(srcDir, dst string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()
	home, _ := os.UserHomeDir()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			continue
		}
		if home != "" {
			data = []byte(strings.ReplaceAll(string(data), home, "<user>"))
		}
		w, err := zw.Create(e.Name())
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
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
	_, err = io.Copy(out, in)
	return err
}

func maskUser(p string) string {
	home, _ := os.UserHomeDir()
	if home != "" {
		return strings.ReplaceAll(p, home, "<user>")
	}
	return p
}

// ---------- update (manual only, plan §77) ----------

type UpdateCheck struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	UpToDate       bool   `json:"upToDate"`
	Note           string `json:"note"`
}

func (a *App) CheckForUpdates() UpdateCheck {
	return UpdateCheck{
		CurrentVersion: a.version,
		LatestVersion:  a.version,
		UpToDate:       true,
		Note:           "V1 使用手动检查更新，当前未配置更新服务器",
	}
}
