package models

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type Util struct {
	namespacePattern *regexp.Regexp
	servicePattern   *regexp.Regexp
	branchPattern    *regexp.Regexp
}

var (
	utilOnce sync.Once
	utilInst *Util
)

func NewUtil() *Util {
	utilOnce.Do(func() {
		utilInst = &Util{
			namespacePattern: regexp.MustCompile("[^a-z0-9-]"),
			servicePattern:   regexp.MustCompile("[^a-z0-9]"),
			branchPattern:    regexp.MustCompile(`[^a-z0-9_-]`),
		}
	})
	return utilInst
}

func (this *Util) normalize(pattern *regexp.Regexp, delim string, repl string, args ...string) string {
	names := make([]string, len(args))
	for i, arg := range args {
		arg = strings.ToLower(arg)
		names[i] = pattern.ReplaceAllLiteralString(arg, repl)
	}
	return strings.Join(names, delim)
}

func (this *Util) Normalize(delim string, args ...string) string {
	return this.normalize(this.servicePattern, delim, "", args...)
}

func (this *Util) NormalizeByHyphen(delim string, args ...string) string {
	return this.normalize(this.namespacePattern, delim, "-", args...)
}

func (this *Util) NormalizeGitBranch(delim string, args ...string) string {
	return this.normalize(this.branchPattern, delim, ".", args...)
}

func (this *Util) Hash(in interface{}) (string, error) {
	out, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	hash32 := fnv.New32a()
	_, err = hash32.Write(out)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash32.Sum32()), nil
}

func (this *Util) TCPPortsToList(in string) ([]int, error) {
	var tcpPorts []int
	for _, tcpPortStr := range strings.Split(in, ",") {
		tcpPortStr = strings.TrimSpace(tcpPortStr)
		if len(tcpPortStr) == 0 {
			continue
		}
		tcpPort, err := strconv.Atoi(tcpPortStr)
		if err != nil {
			return tcpPorts, fmt.Errorf("invalid tcp port %s: %v", tcpPort, err)
		}
		if tcpPort <= 0 || tcpPort > 65535 {
			return tcpPorts, fmt.Errorf("invalid tcp port range %v", tcpPort)
		}
		tcpPorts = append(tcpPorts, tcpPort)
	}
	return tcpPorts, nil
}
