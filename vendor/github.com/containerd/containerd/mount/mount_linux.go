package mount

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/docker/docker/pkg/reexec"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

var pagesize = 4096

func init() {
	pagesize = os.Getpagesize()

	reexec.Register("containerd-mountat", mountAtMain)
}

type mountOption struct {
	Source string
	Target string
	FsType string
	Flags  uintptr
	Data   string
}

// Mount to the provided target path
func (m *Mount) Mount(target string) error {
	var chdir string

	// avoid hitting one page limit of mount argument buffer
	//
	// NOTE: 512 is magic number as buffer.
	if m.Type == "overlay" && lowerdirOptSize(m) > (pagesize-512) {
		chdir, m = compactLowerdirOption(m)
	}

	flags, data := parseMountOptions(m.Options)

	// propagation types.
	const ptypes = unix.MS_SHARED | unix.MS_PRIVATE | unix.MS_SLAVE | unix.MS_UNBINDABLE

	// Ensure propagation type change flags aren't included in other calls.
	oflags := flags &^ ptypes

	// In the case of remounting with changed data (data != ""), need to call mount (moby/moby#34077).
	if flags&unix.MS_REMOUNT == 0 || data != "" {
		// Initial call applying all non-propagation flags for mount
		// or remount with changed data
		if err := mountAt(chdir, m.Source, target, m.Type, uintptr(oflags), data); err != nil {
			return err
		}
	}

	if flags&ptypes != 0 {
		// Change the propagation type.
		const pflags = ptypes | unix.MS_REC | unix.MS_SILENT
		if err := unix.Mount("", target, "", uintptr(flags&pflags), ""); err != nil {
			return err
		}
	}

	const broflags = unix.MS_BIND | unix.MS_RDONLY
	if oflags&broflags == broflags {
		// Remount the bind to apply read only.
		return unix.Mount("", target, "", uintptr(oflags|unix.MS_REMOUNT), "")
	}
	return nil
}

// Unmount the provided mount path with the flags
func Unmount(target string, flags int) error {
	if err := unmount(target, flags); err != nil && err != unix.EINVAL {
		return err
	}
	return nil
}

func unmount(target string, flags int) error {
	for i := 0; i < 50; i++ {
		if err := unix.Unmount(target, flags); err != nil {
			switch err {
			case unix.EBUSY:
				time.Sleep(50 * time.Millisecond)
				continue
			default:
				return err
			}
		}
		return nil
	}
	return errors.Wrapf(unix.EBUSY, "failed to unmount target %s", target)
}

// UnmountAll repeatedly unmounts the given mount point until there
// are no mounts remaining (EINVAL is returned by mount), which is
// useful for undoing a stack of mounts on the same mount point.
func UnmountAll(mount string, flags int) error {
	for {
		if err := unmount(mount, flags); err != nil {
			// EINVAL is returned if the target is not a
			// mount point, indicating that we are
			// done. It can also indicate a few other
			// things (such as invalid flags) which we
			// unfortunately end up squelching here too.
			if err == unix.EINVAL {
				return nil
			}
			return err
		}
	}
}

// parseMountOptions takes fstab style mount options and parses them for
// use with a standard mount() syscall
func parseMountOptions(options []string) (int, string) {
	var (
		flag int
		data []string
	)
	flags := map[string]struct {
		clear bool
		flag  int
	}{
		"async":         {true, unix.MS_SYNCHRONOUS},
		"atime":         {true, unix.MS_NOATIME},
		"bind":          {false, unix.MS_BIND},
		"defaults":      {false, 0},
		"dev":           {true, unix.MS_NODEV},
		"diratime":      {true, unix.MS_NODIRATIME},
		"dirsync":       {false, unix.MS_DIRSYNC},
		"exec":          {true, unix.MS_NOEXEC},
		"mand":          {false, unix.MS_MANDLOCK},
		"noatime":       {false, unix.MS_NOATIME},
		"nodev":         {false, unix.MS_NODEV},
		"nodiratime":    {false, unix.MS_NODIRATIME},
		"noexec":        {false, unix.MS_NOEXEC},
		"nomand":        {true, unix.MS_MANDLOCK},
		"norelatime":    {true, unix.MS_RELATIME},
		"nostrictatime": {true, unix.MS_STRICTATIME},
		"nosuid":        {false, unix.MS_NOSUID},
		"rbind":         {false, unix.MS_BIND | unix.MS_REC},
		"relatime":      {false, unix.MS_RELATIME},
		"remount":       {false, unix.MS_REMOUNT},
		"ro":            {false, unix.MS_RDONLY},
		"rw":            {true, unix.MS_RDONLY},
		"strictatime":   {false, unix.MS_STRICTATIME},
		"suid":          {true, unix.MS_NOSUID},
		"sync":          {false, unix.MS_SYNCHRONOUS},
	}
	for _, o := range options {
		// If the option does not exist in the flags table or the flag
		// is not supported on the platform,
		// then it is a data value for a specific fs type
		if f, exists := flags[o]; exists && f.flag != 0 {
			if f.clear {
				flag &^= f.flag
			} else {
				flag |= f.flag
			}
		} else {
			data = append(data, o)
		}
	}
	return flag, strings.Join(data, ",")
}

// compactLowerdirOption updates overlay lowdir option and returns the common
// dir in the all the lowdirs.
func compactLowerdirOption(m *Mount) (string, *Mount) {
	idx, dirs := findOverlayLowerdirs(m)
	// no need to compact if there is only one lowerdir
	if idx == -1 || len(dirs) == 1 {
		return "", m
	}

	// find out common dir
	commondir := longestCommonPrefix(dirs)
	if commondir == "" {
		return "", m
	}

	// NOTE: the snapshot id is based on digits. in order to avoid to get
	// snapshots/x, need to back to parent snapshots dir. however, there is
	// assumption that the common dir is ${root}/io.containerd.v1.overlayfs.
	commondir = path.Dir(commondir)
	if commondir == "/" {
		return "", m
	}
	commondir = commondir + "/"

	newdirs := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		newdirs = append(newdirs, dir[len(commondir):])
	}

	m.Options = append(m.Options[:idx], m.Options[idx+1:]...)
	m.Options = append(m.Options, fmt.Sprintf("lowerdir=%s", strings.Join(newdirs, ":")))
	return commondir, m
}

// lowerdirOptSize returns the bytes of lowerdir option.
func lowerdirOptSize(m *Mount) int {
	for _, opt := range m.Options {
		if strings.HasPrefix(opt, "lowerdir=") {
			return len(opt)
		}
	}
	return 0
}

// findOverlayLowerdirs returns the index of lowerdir in mount's options and
// all the lowerdir target.
func findOverlayLowerdirs(m *Mount) (int, []string) {
	var (
		idx    = -1
		prefix = "lowerdir="
	)

	for i, opt := range m.Options {
		if strings.HasPrefix(opt, prefix) {
			idx = i
			break
		}
	}

	if idx == -1 {
		return -1, nil
	}
	return idx, strings.Split(m.Options[idx][len(prefix):], ":")
}

// longestCommonPrefix finds the longest common prefix in the string slice.
func longestCommonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	} else if len(strs) == 1 {
		return strs[0]
	}

	// find out the min/max value by alphabetical order
	min, max := strs[0], strs[0]
	for _, str := range strs[1:] {
		if min > str {
			min = str
		}
		if max < str {
			max = str
		}
	}

	// find out the common part between min and max
	for i := 0; i < len(min) && i < len(max); i++ {
		if min[i] != max[i] {
			return min[:i]
		}
	}
	return min
}

// mountAtMain acts like execute binary.
func mountAtMain() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	flag.Parse()
	if err := os.Chdir(flag.Arg(0)); err != nil {
		fatal(err)
	}

	var opt mountOption
	if err := json.NewDecoder(os.Stdin).Decode(&opt); err != nil {
		fatal(err)
	}

	if err := unix.Mount(opt.Source, opt.Target, opt.FsType, opt.Flags, opt.Data); err != nil {
		fatal(err)
	}
	os.Exit(0)
}

// mountAt will re-exec mountAtMain to change work dir if necessary.
func mountAt(chdir string, source, target, ftype string, flags uintptr, data string) error {
	if chdir == "" {
		return unix.Mount(source, target, ftype, flags, data)
	}

	opt := mountOption{
		Source: source,
		Target: target,
		FsType: ftype,
		Flags:  flags,
		Data:   data,
	}

	cmd := reexec.Command("containerd-mountat", chdir)

	w, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("mountat error on open stdin pipe: %v", err)
	}

	out := bytes.NewBuffer(nil)
	cmd.Stdout, cmd.Stderr = out, out

	if err := cmd.Start(); err != nil {
		w.Close()
		return fmt.Errorf("mountat error on start cmd: %v", err)
	}

	if err := json.NewEncoder(w).Encode(opt); err != nil {
		w.Close()
		return fmt.Errorf("mountat json-encode option into pipe failed: %v", err)
	}

	w.Close()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("mountat error: %v: combined output: %v", err, out)
	}
	return nil
}

func fatal(v interface{}) {
	fmt.Fprint(os.Stderr, v)
	os.Exit(1)
}
