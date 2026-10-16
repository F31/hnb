package toolkit

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/F31/hnb/pkg/provision"
)

// UserRun operates as the login account, without sudo (for authorized_keys).
func (p SSHNodeProbe) UserRun(ctx context.Context, node NodeConfig, script string, data io.Reader) (string, error) {
	if !sshUser.MatchString(node.User) || !usableIPv4(node.SSHAddress) {
		return "", fmt.Errorf("SSH account and unicast IPv4 address are required")
	}
	return p.authenticatedRun(ctx, node, script, data, false)
}

func (p SSHNodeProbe) authenticatedRun(ctx context.Context, node NodeConfig, script string, data io.Reader, privileged bool) (string, error) {
	args := []string{"-o", "StrictHostKeyChecking=yes", "-o", "UpdateHostKeys=no", "-o", "IdentityAgent=none", "-o", "IdentitiesOnly=yes", "-o", "ConnectTimeout=10", "-o", "LogLevel=ERROR"}
	program := "ssh"
	var passwordReader, passwordWriter *os.File
	if node.SSHAuth == "password" {
		if p.Password == "" || len(p.Password) > 4096 || strings.ContainsAny(p.Password, "\r\n\x00") {
			return "", fmt.Errorf("a nonempty single-line SSH password is required (maximum 4096 bytes)")
		}
		if _, err := exec.LookPath("sshpass"); err != nil {
			return "", fmt.Errorf("password authentication requires sshpass on the operator host")
		}
		var err error
		passwordReader, passwordWriter, err = os.Pipe()
		if err != nil {
			return "", fmt.Errorf("cannot create SSH credential pipe")
		}
		defer passwordReader.Close()
		defer passwordWriter.Close()
		// Write concurrently: even a small kernel pipe cannot block startup.
		// Closing both descriptors on return also unblocks failed exec paths.
		go func() {
			defer passwordWriter.Close()
			_, _ = io.WriteString(passwordWriter, p.Password+"\n")
		}()
		program = "sshpass"
		args = append([]string{"-d", "3", "ssh"}, args...)
		args = append(args, "-o", "BatchMode=no", "-o", "PreferredAuthentications=password", "-o", "PubkeyAuthentication=no", "-o", "NumberOfPasswordPrompts=1")
	} else {
		args = append(args, "-o", "BatchMode=yes", "-i", node.SSHKeyFile)
	}
	command := "sh -c " + provision.ShQuote(script)
	if privileged {
		sudo := "sudo -n -- "
		if p.SudoPassword != "" {
			if len(p.SudoPassword) > 4096 || strings.ContainsAny(p.SudoPassword, "\r\n\x00") {
				return "", fmt.Errorf("sudo password must be a single line (maximum 4096 bytes)")
			}
			// sudo consumes exactly the credential line before the fixed script reads data.
			sudo = "sudo -k -S -p '' -- "
			command = "IFS= read -r hnb_sudo_password; if [ \"$(id -u)\" -eq 0 ]; then " + command + "; else printf '%s\\n' \"$hnb_sudo_password\" | " + sudo + "true && sudo -n -- " + command + "; fi"
			if data == nil {
				data = strings.NewReader("")
			}
			data = io.MultiReader(strings.NewReader(p.SudoPassword+"\n"), data)
		} else {
			command = "if [ \"$(id -u)\" -eq 0 ]; then " + command + "; else " + sudo + command + "; fi"
		}
	}
	args = append(args, "--", node.User+"@"+node.SSHAddress, command)
	cmd := exec.CommandContext(ctx, program, args...)
	if passwordReader != nil {
		cmd.ExtraFiles = []*os.File{passwordReader}
	}
	cmd.Stdin = data
	output := &probeOutput{}
	cmd.Stdout = output
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("SSH authentication/host verification/remote operation failed")
	}
	return output.String(), nil
}
