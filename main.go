package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const releaseURL = "https://github.com/MHSanaei/3x-ui/releases/latest/download/x-ui-linux-amd64.tar.gz"

const (
	publicPort  = "2053"
	panelPort   = "20530"
	vlessPort   = "20868"
	vlessPrefix = "/xvpnws/"
	subPort     = "2096"
	subPrefix   = "/sub/"
)

func main() {
	installDir := "/app/x-ui"
	binPath := filepath.Join(installDir, "x-ui")

	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		fmt.Println("Downloading official 3x-ui release...")
		if err := downloadAndExtract(releaseURL, "/app"); err != nil {
			fmt.Println("download error:", err)
			os.Exit(1)
		}
	}

	os.Chmod(binPath, 0755)
	filepath.Walk(filepath.Join(installDir, "bin"), func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			os.Chmod(path, 0755)
		}
		return nil
	})

	dataDir := "/app/data"
	os.MkdirAll(dataDir, 0755)
	os.MkdirAll(filepath.Join(dataDir, "logs"), 0755)

	cmd := exec.Command(binPath)
	cmd.Dir = installDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"XUI_DB_FOLDER="+dataDir,
		"XUI_LOG_FOLDER="+filepath.Join(dataDir, "logs"),
		"XUI_PORT="+panelPort,
	)

	if err := cmd.Start(); err != nil {
		fmt.Println("failed to start x-ui:", err)
		os.Exit(1)
	}

	go startProxy()

	if err := cmd.Wait(); err != nil {
		fmt.Println("run error:", err)
		os.Exit(1)
	}
}

func startProxy() {
	time.Sleep(3 * time.Second)

	panelTarget, _ := url.Parse("http://127.0.0.1:" + panelPort)
	vlessTarget, _ := url.Parse("http://127.0.0.1:" + vlessPort)
	subTarget, _ := url.Parse("http://127.0.0.1:" + subPort)

	panelProxy := httputil.NewSingleHostReverseProxy(panelTarget)
	vlessProxy := httputil.NewSingleHostReverseProxy(vlessTarget)
	subProxy := httputil.NewSingleHostReverseProxy(subTarget)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, vlessPrefix):
			vlessProxy.ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, subPrefix):
			subProxy.ServeHTTP(w, r)
		default:
			panelProxy.ServeHTTP(w, r)
		}
	})

	fmt.Println("Reverse proxy listening on :" + publicPort)
	if err := http.ListenAndServe(":"+publicPort, mux); err != nil {
		fmt.Println("proxy error:", err)
	}
}

func downloadAndExtract(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("bad status: %s", resp.Status)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}
