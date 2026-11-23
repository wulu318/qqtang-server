package networksetup

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The 5.2 client opens the retired family-rank page from uiMain.pyc. Rewriting
// that embedded URL (same byte length, so the marshalled string stays valid)
// points the existing 家族赛 web panel at our own leaderboard without
// recompiling Python 2.3 bytecode.
const embeddedFamilyRankURL = "http%3A%2F%2Fapp.qqtang.qq.com%2Fcgi-bin%2Fa20071119jzls%2Fmy_family_rank.cgi"

func patchClientWebURLs(clientRoot, host string, directoryPort int) error {
	path := filepath.Join(clientRoot, "res", "uiRes", "uiMain.pyc")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	base := "http%3A%2F%2F" + host + "%3A" + strconv.Itoa(directoryPort) + "%2Fleaderboard"
	if len(base) > len(embeddedFamilyRankURL) {
		return fmt.Errorf("排行榜地址过长：%d > %d", len(base), len(embeddedFamilyRankURL))
	}
	padding := len(embeddedFamilyRankURL) - len(base)
	replacement := base
	if padding > 0 {
		marker := "?pad="
		if padding < len(marker) {
			marker = "?"
		}
		replacement += marker + strings.Repeat("a", padding-len(marker))
	}
	if len(replacement) != len(embeddedFamilyRankURL) {
		return fmt.Errorf("内嵌 URL 长度不一致：%d != %d", len(replacement), len(embeddedFamilyRankURL))
	}
	if bytes.Contains(data, []byte(base)) {
		return nil
	}
	if !bytes.Contains(data, []byte(embeddedFamilyRankURL)) {
		return fmt.Errorf("uiMain.pyc 中找不到原始家族赛地址")
	}
	updated := bytes.ReplaceAll(data, []byte(embeddedFamilyRankURL), []byte(replacement))
	return replaceFile(path, updated)
}
