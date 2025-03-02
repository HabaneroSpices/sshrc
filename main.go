package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
)

const maxSize = 65536 // 64KB limit

func encodeFileToBase64(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(content), nil
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
		fmt.Println("Usage: sshrc <host> [ssh options]")
		os.Exit(1)
	}

	sshrcEncoded, err := encodeFileToBase64(sshrcPath)
	if err != nil {
		fmt.Println("Failed to encode .sshrc:", err)
		os.Exit(1)
	}

	if len(sshrcEncoded) > maxSize {
		fmt.Println("Maximum size(encoded) reached for .sshrc: >", maxSize)
		os.Exit(1)
	}

	bashrcContent := `
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
	exec bash --rcfile $SSHHOME/sshrc.bashrc
	`

	sshCommand := fmt.Sprintf(tmpScript, sshrcEncoded, bashrcEncoded)

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
