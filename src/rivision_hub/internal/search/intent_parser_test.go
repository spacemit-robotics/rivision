package search

import (
	"testing"
	"time"
)

var testCameras = []CameraInfo{
	{ID: "cam_entrance", Name: "入口", Location: "一楼大厅"},
	{ID: "cam_warehouse", Name: "仓库门口", Location: "后院"},
	{ID: "cam_cashier", Name: "收银台", Location: "一楼"},
	{ID: "cam_parking", Name: "停车场", Location: "地下一层"},
}

var testNow = time.Date(2026, 5, 6, 14, 30, 0, 0, time.Local)

func TestParseTimeToday(t *testing.T) {
	p := NewIntentParser("", false)
	intent := p.Parse("今天有没有人打架", testCameras, testNow)

	if intent.TimeStart == nil || intent.TimeEnd == nil {
		t.Fatal("expected time range for '今天'")
	}
	if intent.TimeStart.Day() != 6 {
		t.Errorf("start day = %d, want 6", intent.TimeStart.Day())
	}
	if intent.Behavior != "fighting" {
		t.Errorf("behavior = %q, want 'fighting'", intent.Behavior)
	}
}

func TestParseTimeYesterdayAfternoon(t *testing.T) {
	p := NewIntentParser("", false)
	intent := p.Parse("昨天下午在仓库门口徘徊的人", testCameras, testNow)

	if intent.TimeStart == nil {
		t.Fatal("expected time range for '昨天下午'")
	}
	if intent.TimeStart.Day() != 5 {
		t.Errorf("start day = %d, want 5", intent.TimeStart.Day())
	}
	if intent.TimeStart.Hour() != 12 {
		t.Errorf("start hour = %d, want 12", intent.TimeStart.Hour())
	}
	if intent.TimeEnd.Hour() != 18 {
		t.Errorf("end hour = %d, want 18", intent.TimeEnd.Hour())
	}

	// Camera matching
	if len(intent.CameraIDs) != 1 || intent.CameraIDs[0] != "cam_warehouse" {
		t.Errorf("cameras = %v, want [cam_warehouse]", intent.CameraIDs)
	}

	if intent.Behavior != "loitering" {
		t.Errorf("behavior = %q, want 'loitering'", intent.Behavior)
	}
}

func TestParseTimeRecentHours(t *testing.T) {
	p := NewIntentParser("", false)
	intent := p.Parse("最近3小时入口有人闯入吗", testCameras, testNow)

	if intent.TimeStart == nil {
		t.Fatal("expected time range for '最近3小时'")
	}
	diff := testNow.Sub(*intent.TimeStart)
	if diff < 2*time.Hour || diff > 4*time.Hour {
		t.Errorf("time range = %v, want ~3h", diff)
	}

	if len(intent.CameraIDs) != 1 || intent.CameraIDs[0] != "cam_entrance" {
		t.Errorf("cameras = %v, want [cam_entrance]", intent.CameraIDs)
	}

	if intent.Behavior != "intrusion" {
		t.Errorf("behavior = %q, want 'intrusion'", intent.Behavior)
	}
}

func TestParseHourRange(t *testing.T) {
	p := NewIntentParser("", false)
	intent := p.Parse("9点到12点收银台有什么异常", testCameras, testNow)

	if intent.TimeStart == nil {
		t.Fatal("expected time range for '9点到12点'")
	}
	if intent.TimeStart.Hour() != 9 {
		t.Errorf("start hour = %d, want 9", intent.TimeStart.Hour())
	}
	if intent.TimeEnd.Hour() != 12 {
		t.Errorf("end hour = %d, want 12", intent.TimeEnd.Hour())
	}
	if len(intent.CameraIDs) != 1 || intent.CameraIDs[0] != "cam_cashier" {
		t.Errorf("cameras = %v, want [cam_cashier]", intent.CameraIDs)
	}
}

func TestParseNoTimeNoCam(t *testing.T) {
	p := NewIntentParser("", false)
	intent := p.Parse("穿红色衣服的人", testCameras, testNow)

	if intent.TimeStart != nil {
		t.Error("should not have time range")
	}
	if len(intent.CameraIDs) != 0 {
		t.Errorf("cameras = %v, want empty", intent.CameraIDs)
	}
	if intent.SearchText == "" {
		t.Error("search_text should not be empty")
	}
	if intent.Method != "regex" {
		t.Errorf("method = %q, want 'regex'", intent.Method)
	}
}

func TestParseCameraLocation(t *testing.T) {
	p := NewIntentParser("", false)
	intent := p.Parse("地下一层有车辆异常停放", testCameras, testNow)

	if len(intent.CameraIDs) != 1 || intent.CameraIDs[0] != "cam_parking" {
		t.Errorf("cameras = %v, want [cam_parking]", intent.CameraIDs)
	}
}

func TestStartOfDay(t *testing.T) {
	d := startOfDay(testNow)
	if d.Hour() != 0 || d.Minute() != 0 {
		t.Errorf("startOfDay = %v", d)
	}
}

func TestParseInt(t *testing.T) {
	if v := parseInt("3", 0); v != 3 {
		t.Errorf("parseInt('3') = %d", v)
	}
	if v := parseInt("", 5); v != 5 {
		t.Errorf("parseInt('') = %d, want 5", v)
	}
}

func TestCleanSearchText(t *testing.T) {
	result := cleanSearchText("穿红色衣服的人")
	if result != "穿红色衣服人" {
		t.Errorf("cleaned = %q", result)
	}
}
