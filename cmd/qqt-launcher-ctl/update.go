package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type localVersion struct {
	Version string `json:"version"`
}

func versionPath(root string) string {
	return filepath.Join(root, "runtime", "version.json")
}

func readLocalVersion(root string) string {
	data, err := os.ReadFile(versionPath(root))
	if err != nil {
		return ""
	}
	var version localVersion
	if json.Unmarshal(data, &version) != nil {
		return ""
	}
	return version.Version
}

// runUpdate checks the bootstrap document, downloads a package, verifies its
// hash and replaces everything except saves, logs and backups.
func runUpdate(root, configURL string) (bool, string, error) {
	remote, err := fetchRemoteConfig(root, configURL, 10*time.Second)
	if err != nil {
		return false, "", err
	}
	if remote.Version == "" {
		return false, "", fmt.Errorf("配置缺少 version")
	}
	if readLocalVersion(root) == remote.Version {
		return false, "已是最新版本 " + remote.Version, nil
	}
	if remote.PackageURL == "" {
		return false, "", fmt.Errorf("配置缺少 package_url")
	}
	updates := filepath.Join(root, "runtime", "updates")
	if err = os.MkdirAll(updates, 0o755); err != nil {
		return false, "", err
	}
	archive := filepath.Join(updates, "package-"+remote.Version+".zip")
	if err = download(remote.PackageURL, archive); err != nil {
		return false, "", err
	}
	if remote.SHA256 != "" {
		if err = verifySHA256(archive, remote.SHA256); err != nil {
			return false, "", err
		}
	}
	temp, err := os.MkdirTemp(updates, "extract-*")
	if err != nil {
		return false, "", err
	}
	defer os.RemoveAll(temp)
	if err = unzip(archive, temp); err != nil {
		return false, "", err
	}
	source := temp
	if entries, readErr := os.ReadDir(temp); readErr == nil && len(entries) == 1 && entries[0].IsDir() {
		source = filepath.Join(temp, entries[0].Name())
	}
	if err = replaceTree(source, root); err != nil {
		return false, "", err
	}
	if err = os.WriteFile(versionPath(root), []byte(fmt.Sprintf("{\"version\":%q}\n", remote.Version)), 0o644); err != nil {
		return false, "", err
	}
	return true, "已更新到 " + remote.Version, nil
}

func download(url, target string) error {
	client := &http.Client{Timeout: 15 * time.Minute}
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败：%s", response.Status)
	}
	file, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err = io.Copy(file, response.Body); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func verifySHA256(path, want string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return err
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(got, strings.TrimSpace(want)) {
		return fmt.Errorf("安装包校验失败：%s != %s", got, want)
	}
	return nil
}

func unzip(archive, target string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, entry := range reader.File {
		destination := filepath.Join(target, filepath.Clean(entry.Name))
		if !strings.HasPrefix(destination, filepath.Clean(target)+string(os.PathSeparator)) {
			return fmt.Errorf("安装包包含非法路径")
		}
		if entry.FileInfo().IsDir() {
			if err = os.MkdirAll(destination, 0o755); err != nil {
				return err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		source, openErr := entry.Open()
		if openErr != nil {
			return openErr
		}
		out, createErr := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, entry.Mode())
		if createErr != nil {
			source.Close()
			return createErr
		}
		if _, err = io.Copy(out, source); err != nil {
			out.Close()
			source.Close()
			return err
		}
		out.Close()
		source.Close()
	}
	return nil
}

var protectedPrefixes = []string{
	string(os.PathSeparator) + "runtime" + string(os.PathSeparator) + "data",
	string(os.PathSeparator) + "runtime" + string(os.PathSeparator) + "backups",
	string(os.PathSeparator) + "runtime" + string(os.PathSeparator) + "logs",
	string(os.PathSeparator) + "runtime" + string(os.PathSeparator) + "updates",
	string(os.PathSeparator) + "runtime" + string(os.PathSeparator) + "captures",
}

func isProtected(relative string) bool {
	path := string(os.PathSeparator) + relative
	for _, prefix := range protectedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func replaceTree(source, root string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, relErr := filepath.Rel(source, path)
		if relErr != nil || relative == "." {
			return nil
		}
		destination := filepath.Join(root, relative)
		if isProtected(relative) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		if err = os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		input, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		output, createErr := os.Create(destination)
		if createErr != nil {
			input.Close()
			return createErr
		}
		if _, err = io.Copy(output, input); err != nil {
			output.Close()
			input.Close()
			return err
		}
		output.Close()
		return input.Close()
	})
}
