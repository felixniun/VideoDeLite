package media

import (
	"encoding/json"
	"testing"
)

// Regression: DJI drone mp4 files emit side_data_list.rotation as a NUMBER,
// which crashed the parser when it was declared as a string
// ("cannot unmarshal number into Go struct field ... rotation of type string").
func TestRotationNumberAndString(t *testing.T) {
	var out ffprobeOutput
	// rotation as number (DJI case)
	numJSON := `{"streams":[{"codec_type":"video","codec_name":"h264","side_data_list":[{"side_data_type":"Display Matrix","rotation":-90}]}]}`
	if err := json.Unmarshal([]byte(numJSON), &out); err != nil {
		t.Fatalf("number rotation failed to parse: %v", err)
	}
	if got := sideDataString(out.Streams[0].SideDataList[0].Rotation); got != "-90" {
		t.Fatalf("number rotation normalized to %q, want -90", got)
	}

	// rotation as string (other ffprobe builds)
	var out2 ffprobeOutput
	strJSON := `{"streams":[{"codec_type":"video","side_data_list":[{"side_data_type":"Display Matrix","rotation":"270"}]}]}`
	if err := json.Unmarshal([]byte(strJSON), &out2); err != nil {
		t.Fatalf("string rotation failed to parse: %v", err)
	}
	if got := sideDataString(out2.Streams[0].SideDataList[0].Rotation); got != "270" {
		t.Fatalf("string rotation normalized to %q, want 270", got)
	}
}
