package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const maxSize = 65536 // 64KB limit

func encodeFileToBase64(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(content), nil
}

func compressDirectory(directory string) (*bytes.Buffer, error) {
	buffer := new(bytes.Buffer)
	gzipWriter := gzip.NewWriter(buffer)
	tarWriter := tar.NewWriter(gzipWriter)

	err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if _, err := io.Copy(tarWriter, file); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := tarWriter.Close(); err != nil {
		return nil, err
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, err
	}
	return buffer, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: sshrc <host> [ssh options]")
		os.Exit(1)
	}

	sshHome := os.Getenv("SSHHOME")
	if sshHome == "" {
		sshHome = os.Getenv("HOME")
	}

	sshrcPath := sshHome + "/.sshrc"
	if _, err := os.Stat(sshrcPath); os.IsNotExist(err) {
		fmt.Println("Failed to open .sshrc:", err)
		os.Exit(1)
	}

	var sshrcDEncoded string

	sshrcDPath := sshHome + "/.sshrc.d"
	if _, err := os.Stat(sshrcDPath); !os.IsNotExist(err) {
		sshrcDCompressedBuffer, err := compressDirectory(sshrcDPath)
		if err != nil {
			panic(err)
		}
		sshrcDEncoded = base64.StdEncoding.EncodeToString(sshrcDCompressedBuffer.Bytes())
	} else {
		sshrcDEncoded = ""
	}

	sshrcEncoded, err := encodeFileToBase64(sshrcPath)
	if err != nil {
		fmt.Println("Failed to encode .sshrc:", err)
		os.Exit(1)
	}

	if len(sshrcEncoded)+len(sshrcDEncoded) > maxSize {
		fmt.Printf("Error: Combined size of .sshrc (%.2f KB) and .sshrc.d/ (%.2f KB) exceeds the maximum allowed size of %.f KB.\n",
			float64(len(sshrcEncoded))/1024, float64(len(sshrcDEncoded))/1024, float64(maxSize)/1024)
		os.Exit(1)
	}

	bashrcContent := `
		find /tmp -maxdepth 1 -type d -wholename "/tmp/.$USER.sshrc.*" ! -wholename "$SSHHOME"  | xargs -I% rm -r "%"
		trap "rm -rf $SSHRCCLEANUP; exit" 0
		printf "\033]0; $(hostname --short)\007" > /dev/tty
		if [ -r /etc/profile ]; then source /etc/profile; fi
		if [ -r ~/.bash_profile ]; then source ~/.bash_profile
		elif [ -r ~/.bash_login ]; then source ~/.bash_login
		elif [ -r ~/.profile ]; then source ~/.profile
		fi
		export PATH=$PATH:$SSHHOME
		source $SSHHOME/sshrc;
	`

	bashrcEncoded := base64.StdEncoding.EncodeToString([]byte(bashrcContent))

	tmpScript := `
	if [ ! -e ~/.hushlogin ]; then
		if [ -e /etc/motd ]; then cat /etc/motd; fi
		if [ -e /etc/update-motd.d ]; then run-parts /etc/update-motd.d/ 2>/dev/null; fi
		last -F $USER 2>/dev/null | grep -v 'still logged in' | head -n1 | awk '{print "Last login:",$4,$5,"",$6,$7,$8,"from",$3;}'
	fi
	export SSHHOME=$(mktemp -d -t .$(whoami).sshrc.XXXX)
	export SSHRCCLEANUP=$SSHHOME
	trap "rm -rf $SSHRCCLEANUP; exit" 0
	echo '%s' | base64 --decode > $SSHHOME/sshrc
	echo '%s' | base64 --decode > $SSHHOME/sshrc.bashrc
	echo '%s' | base64 --decode | tar mxzf - -C $SSHHOME 2>/dev/null
	exec bash --rcfile $SSHHOME/sshrc.bashrc
	`

	sshCommand := fmt.Sprintf(tmpScript, sshrcEncoded, bashrcEncoded, sshrcDEncoded)

	sshArgs := os.Args[1:]
	cmdArgs := append(sshArgs, "-t", "--", sshCommand)
	cmd := exec.Command("ssh", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		fmt.Println("SSH execution error:", err)
		os.Exit(1)
	}
}
