package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

const vietnameseLetters = "ăâđêôơưĂÂĐÊÔƠƯáàảãạấầẩẫậắằẳẵặéèẻẽẹếềểễệíìỉĩịóòỏõọốồổỗộớờởỡợúùủũụứừửữựýỳỷỹỵÁÀẢÃẠẤẦẨẪẬẮẰẲẴẶÉÈẺẼẸẾỀỂỄỆÍÌỈĨỊÓÒÕỌỐỒỔỖỘỚỜỞỠỢÚÙỦŨỤỨỪỬỮỰÝỲỶỸỴ"

func TestBuiltInBrowserUIUsesEnglish(t *testing.T) {
	if strings.ContainsAny(indexHTML+appJS, vietnameseLetters) {
		t.Fatal("core browser UI still contains Vietnamese text")
	}
	assets := []string{
		"features.js",
		"featuremods/all.js",
		"featuremods/appearance.js",
		"featuremods/clearfiles.js",
		"featuremods/envprofiles.js",
		"featuremods/gitstatus.js",
		"featuremods/inputs.js",
		"featuremods/menus.js",
		"featuremods/notifications.js",
		"featuremods/pagetitle.js",
		"featuremods/ptvpriority.js",
		"featuremods/rename.js",
		"featuremods/restartclear.js",
		"featuremods/selfupdate.js",
		"featuremods/sidebar.js",
		"featuremods/split.js",
		"featuremods/terminalcwd.js",
		"featuremods/terminalrestore.js",
		"featuremods/upload.js",
	}
	for _, path := range assets {
		data, err := webassets.Files.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.ContainsAny(string(data), vietnameseLetters) {
			t.Fatalf("built-in browser asset %s still contains Vietnamese text", path)
		}
	}
}
