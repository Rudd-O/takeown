package main

import "bytes"
import "fmt"
import "io"
import "io/ioutil"
import "path/filepath"
import "os"
import "os/exec"
import "strings"
import "syscall"
import "testing"

type Privilege int

const (
	Unprivileged Privilege = iota
	Privileged
)

type Criterion string

const (
	Out  Criterion = "stdout"
	Err  Criterion = "stderr"
	Exit Criterion = "exit code"
)

type Comparator string

const (
	Equals Comparator = "equals"
)

type FileType string

const (
	File      FileType = "file"
	Directory FileType = "directory"
	Symlink   FileType = "symlink"
)

type Expectation struct {
	criterion  Criterion
	comparator Comparator
	value      interface{}
}

type Request struct {
	filetype FileType
	path     string
	owner    uint32
	group    uint32
	mode     os.FileMode
}

type StatInfo struct {
	path  string
	owner *uint32
	group *uint32
	mode  *os.FileMode
}

type RunResult struct {
	out  string
	err  string
	exit int
	v    *TestingVM
}

type TestingVM struct {
	dir                   string
	blockdevForProgram    string
	blockdevForTestData   string
	mountpointForProgram  string
	mountpointForTestData string
	takeownPath           string
	lastDescription       string
	t                     *testing.T
	unprivilegedUser      string
	unprivilegedUid       uint32
}

func That(criterion Criterion, comparator Comparator, value interface{}) Expectation {
	var expectation Expectation
	expectation.criterion = criterion
	expectation.comparator = comparator
	expectation.value = value
	return expectation
}

func Print(stdout string, args ...interface{}) Expectation {
	if len(args) != 0 {
		stdout = fmt.Sprintf(stdout, args...)
	}
	return That(Out, Equals, stdout)
}

func PrintErr(stderr string, args ...interface{}) Expectation {
	if len(args) != 0 {
		stderr = fmt.Sprintf(stderr, args...)
	}
	return That(Err, Equals, stderr)
}

func ExitWith(retval int) Expectation {
	return That(Exit, Equals, retval)
}

func Succeed() Expectation {
	return ExitWith(0)
}

func SucceedQuietly() []Expectation {
	return []Expectation{Print(""), PrintErr(""), Succeed()}
}

func ExitWithUsage() []Expectation {
	return []Expectation{Print(""), PrintErr(USAGE), ExitWith(Usage)}
}

func parseStatBits(s *StatInfo, bits ...uint32) {
	for bitcounter, bit := range bits {
		if bitcounter > 2 {
			break
		}
		if bitcounter == 0 {
			s.owner = new(uint32)
			*s.owner = bit
		} else if bitcounter == 1 {
			s.group = new(uint32)
			*s.group = bit
		} else if bitcounter == 2 {
			s.mode = new(os.FileMode)
			*s.mode = os.FileMode(bit)
		}
	}
}

func Stat(path string, bits ...uint32) StatInfo {
	var s StatInfo
	s.path = path
	parseStatBits(&s, bits...)
	return s
}

func parseRequestBits(r *Request, bits ...uint32) {
	for bitcounter, bit := range bits {
		if bitcounter > 2 {
			break
		}
		if bitcounter == 0 {
			r.owner = bit
		} else if bitcounter == 1 {
			r.group = bit
		} else if bitcounter == 2 {
			r.mode = os.FileMode(bit)
		}
	}
}

func D(path string, bits ...uint32) Request {
	var r Request
	r.path = path
	r.filetype = Directory
	r.mode = 0755
	parseRequestBits(&r, bits...)
	return r
}

func F(path string, bits ...uint32) Request {
	var r Request
	r.path = path
	r.filetype = File
	r.mode = 0644
	parseRequestBits(&r, bits...)
	return r
}

func runAndPrintErrors(command string, args ...string) error {
	c := exec.Command(command, args...)
	var out bytes.Buffer
	c.Stdout = &out
	c.Stderr = &out
	e := c.Run()
	if e != nil {
		fmt.Fprintf(os.Stderr, "%s", out.String())
	}
	return e
}

func copy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	cerr := out.Close()
	if err != nil {
		return err
	}
	return cerr
}

func (t TestingVM) String() string {
	return fmt.Sprintf("VM at %s", t.dir)
}

// Instantiate tries to instantiate a testing space.  If it fails, it returns
// the error, and its partial progress.  The caller must call .Destroy() on
// the testing space to release all resources, irrespective of whether an
// error was returned.
func Instantiate(t *testing.T, unprivilegedUser string) (TestingVM, error) {
	var v TestingVM
	v.lastDescription = "initializing"
	v.t = t

	if unprivilegedUser == "" {
		return v, fmt.Errorf("you need to specify an unprivileged user which must exist")
	}
	unprivilegedUid, err := userToUid(PotentialUsername(unprivilegedUser))
	if err != nil {
		return v, fmt.Errorf("user %q is not known to the system", unprivilegedUser)
	}
	v.unprivilegedUser = unprivilegedUser
	v.unprivilegedUid = uint32(unprivilegedUid)

	tempdir, err := ioutil.TempDir("", "golang-takeown-test")
	if err != nil {
		return v, err
	}
	err = os.Chown(tempdir, int(unprivilegedUid), 0)
	if err != nil {
		return v, err
	}

	v.dir = tempdir
	f, err := os.Create(filepath.Join(tempdir, "takeown-program.vol"))
	if err != nil {
		return v, err
	}
	err = f.Truncate(64 * 1024 * 1024)
	if err != nil {
		return v, err
	}
	err = os.Mkdir(filepath.Join(tempdir, "takeown-program.dir"), 0755)
	if err != nil {
		return v, err
	}

	g, err := os.Create(filepath.Join(tempdir, "takeown-data.vol"))
	if err != nil {
		return v, err
	}
	err = g.Truncate(64 * 1024 * 1024)
	if err != nil {
		return v, err
	}
	err = os.Mkdir(filepath.Join(tempdir, "takeown-data.dir"), 0755)
	if err != nil {
		return v, err
	}

	err = runAndPrintErrors("mkfs.ext2", "-F", f.Name())
	if err != nil {
		return v, err
	}
	err = runAndPrintErrors("mkfs.ext2", "-F", g.Name())
	if err != nil {
		return v, err
	}

	err = runAndPrintErrors("mount", "-o", "loop", f.Name(), filepath.Join(tempdir, "takeown-program.dir"))
	if err != nil {
		return v, err
	}
	v.blockdevForProgram = f.Name()
	v.mountpointForProgram = filepath.Join(tempdir, "takeown-program.dir")
	err = runAndPrintErrors("mount", "-o", "loop", g.Name(), filepath.Join(tempdir, "takeown-data.dir"))
	if err != nil {
		return v, err
	}
	v.blockdevForTestData = g.Name()
	v.mountpointForTestData = filepath.Join(tempdir, "takeown-data.dir")

	if err = copy("takeown", filepath.Join(v.mountpointForProgram, "takeown")); err != nil {
		return v, err
	}
	if err = os.Lchown(filepath.Join(v.mountpointForProgram, "takeown"), 0, 0); err != nil {
		return v, err
	}
	if err = os.Chmod(filepath.Join(v.mountpointForProgram, "takeown"), 0755); err != nil {
		return v, err
	}
	if err = exec.Command("chmod", "u+s", "--", filepath.Join(v.mountpointForProgram, "takeown")).Run(); err != nil {
		return v, err
	}
	v.takeownPath = filepath.Join(v.mountpointForProgram, "takeown")
	v.lastDescription = "initialized"
	return v, nil
}

func (t TestingVM) Destroy() error {
	var err error
	if t.blockdevForProgram != "" {
		e := runAndPrintErrors("umount", "-f", t.blockdevForProgram)
		if err == nil {
			err = e
		}
	}
	if t.blockdevForTestData != "" {
		e := runAndPrintErrors("umount", "-f", t.blockdevForTestData)
		if err == nil {
			err = e
		}
	}
	e := os.RemoveAll(t.dir)
	if err == nil {
		err = e
	}
	return err
}

func (v TestingVM) Datadir() string {
	return v.mountpointForTestData
}

func (v TestingVM) List() error {
	c := exec.Command("ls", "-lRa", v.mountpointForTestData)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// assertPathWithinTestData evaluates the passed path and returns an error
// if the path is not within the test data directory.  Otherwise, it will
// return a string with the full absolutized path.
func (v TestingVM) assertPathWithinTestData(p string) (string, error) {
	fullpath := filepath.Join(v.mountpointForTestData, p)
	ok, err := contains(AbsoluteResolvedPathname(v.mountpointForTestData), AbsoluteResolvedPathname(fullpath))
	if err != nil {
		return "", fmt.Errorf("problem reasoning whether path %q is outside of test directory %q", fullpath, v.mountpointForTestData)
	}
	if !ok {
		if v.mountpointForTestData != fullpath {
			return "", fmt.Errorf("cowardly refusing to manipulate path %q outside of test directory %q", fullpath, v.mountpointForTestData)
		}
	}
	return fullpath, nil
}

// invoke returns the standard output, standard error of a command as it
// exits, along with its integer return status.  If the command could not
// be executed, it returns exit status -256.  If the command experienced an
// unusual exit condition, then the exit status will be -257.
func (t TestingVM) invoke(opts []string, paths []string, privilege Privilege) (stdout string, stderr string, exitcode int, malfunction error) {
	for _, p := range paths {
		_, malfunction = t.assertPathWithinTestData(p)
		if malfunction != nil {
			return "", "", 0, malfunction
		}
	}
	args := append(opts, paths...)
	var c *exec.Cmd
	if privilege == Privileged {
		c = exec.Command(t.takeownPath, args...)
	} else {
		c = exec.Command("runuser", append(
				[]string{"-u", t.unprivilegedUser, "--", t.takeownPath},
				args...
			)...
		)
	}
	c.Dir = t.mountpointForTestData
	var out bytes.Buffer
	var err bytes.Buffer
	c.Stdout = &out
	c.Stderr = &err
	execret := c.Run()
	if exiterr, ok := execret.(*exec.ExitError); ok {
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			exitcode = status.ExitStatus()
		} else {
			malfunction = exiterr
		}
	} else {
		malfunction = execret
	}
	return out.String(), err.String(), exitcode, malfunction
}

// Run runs a program and saves the result in the returned RunResult.  If
// privileges is unspecified, it assumes privileged execution.
func (v *TestingVM) Run(action string, opts []string, paths []string, privileges ...Privilege) *RunResult {
	v.lastDescription = action
	var privilege Privilege
	if len(privileges) > 0 {
		privilege = privileges[0]
	} else {
		privilege = Privileged
	}
	out, err, exit, malfunction := v.invoke(opts, paths, privilege)
	if malfunction != nil {
		v.t.Fatalf("while %s: malfunction invoking takeown: %v", action, malfunction)
	}
	return &RunResult{out, err, exit, v}
}

// Modify modifies files on disk based on a specification passed to it
func (v *TestingVM) Modify(description string, requests ...Request) {
	t := v.t
	v.lastDescription = description
	for _, request := range requests {
		fullpath, assertion := v.assertPathWithinTestData(request.path)
		if assertion != nil {
			t.Fatalf("while %s: %v", description, assertion)
		}
		switch request.filetype {
		case Directory:
			err := os.Mkdir(fullpath, os.FileMode(request.mode))
			if err != nil {
				t.Fatalf("while %s: cannot create %s %q: %v", description, request.filetype, request.path, err)
			}
		case File:
			f, err := os.OpenFile(fullpath, os.O_CREATE|os.O_TRUNC, request.mode)
			if err != nil {
				t.Fatalf("while %s: cannot create %s %q: %v", description, request.filetype, request.path, err)
			}
			f.Close()
		default:
			t.Fatalf("while %s: invalid file type %s for requested path %q", description, request.filetype, request.path)
		}
		err := os.Lchown(fullpath, int(request.owner), int(request.group))
		if err != nil {
			t.Fatalf("while %s: cannot chown %s %q: %v", description, request.filetype, request.path, err)
		}
		if request.filetype != Symlink {
			err := os.Chmod(fullpath, request.mode)
			if err != nil {
				t.Fatalf("while %s: cannot chmod %s %q: %v", description, request.filetype, request.path, err)
			}
		} else {
			if request.mode != os.FileMode(0777) {
				t.Fatalf("while %s: cannot chmod %s %q: %v", description, request.filetype, request.path, err)
			}
		}
	}
}

// Check checks particular files to see if their attributes match.
func (v *TestingVM) Check(stats ...StatInfo) {
	t := v.t
	for _, s := range stats {
		fullpath, assertion := v.assertPathWithinTestData(s.path)
		if assertion != nil {
			t.Fatalf("after %s: %v", v.lastDescription, assertion)
		}
		var statdata syscall.Stat_t
		err := syscall.Lstat(fullpath, &statdata)
		if err != nil {
			t.Fatalf("after %s: cannot stat %q: %v", v.lastDescription, s.path, err)
		}
		if s.owner != nil {
			if statdata.Uid != uint32(*s.owner) {
				t.Errorf("after %s: owner of %q, expected %d, got %d", v.lastDescription, s.path, *s.owner, statdata.Uid)
			}
		}
		if s.group != nil {
			if statdata.Gid != uint32(*s.group) {
				t.Errorf("after %s: group of %q, expected %d, got %d", v.lastDescription, s.path, *s.group, statdata.Gid)
			}
		}
		if s.mode != nil {
			m := os.FileMode(statdata.Mode & 07777)
			if m != *s.mode {
				t.Errorf("after %s: mode of %q, expected %o, got %o", v.lastDescription, s.path, *s.mode, m)
			}
		}
	}
}

// Must expects a particular output or error.  If the expectations do not match,
// it fails the test.
func (r *RunResult) Must(expectations ...Expectation) *RunResult {
	v := r.v
	action := v.lastDescription
	t := v.t
	out := r.out
	err := r.err
	exit := r.exit
	for _, e := range expectations {
		switch expected := e.value.(type) {
		case string:
			// string comparison
			var got string
			switch e.criterion {
			case Out:
				got = out
			case Err:
				got = err
			default:
				t.Fatalf("while %s: invalid criterion %q in expectation %v", action, e.criterion, e)
			}
			switch e.comparator {
			case Equals:
				if got != expected && strings.TrimRight(got, "\n") != expected {
					t.Errorf("while %s: got %s %q, expected %q", action, e.criterion, got, expected)
				}
			default:
				t.Fatalf("while %s: invalid comparator %q in expectation %v", action, e.comparator, e)
			}
		case int:
			// int comparison
			var got int
			switch e.criterion {
			case Exit:
				got = exit
			default:
				t.Fatalf("while %s: invalid criterion %q in expectation %v", action, e.criterion, e)
			}
			switch e.comparator {
			case Equals:
				if got != expected {
					t.Errorf("while %s: got %s %d, expected %d", action, e.criterion, got, expected)
				}
			default:
				t.Fatalf("while %s: invalid comparator %q in expectation %v", action, e.comparator, e)
			}

		default:
			t.Fatalf("while %s: invalid expected value passed to Ensure(): %T %v", action, e.value, e.value)
		}
	}
	return r
}

// Causes expects particular modifications to files.  If the expectations do
// not match, it fails the test.
func (r *RunResult) Causes(stats ...StatInfo) *RunResult {
	v := r.v
	v.Check(stats...)
	return r
}

func init() {
	if os.Getuid() != 0 {
		fmt.Fprintf(os.Stderr, "error: this battery of tests must be run as root\n")
		os.Exit(16)
	}
}

func i(t *testing.T) TestingVM {
	vm, err := Instantiate(t, "nobody")
	if err != nil {
		err2 := vm.Destroy()
		if err2 != nil {
			t.Fatalf("%s could not be instantiated: %v -- an additional error took place while destroying it: %v", vm, err, err2)
		}
		t.Fatalf("%s could not be instantiated: %v", vm, err)
	}
	return vm
}
func d(v TestingVM) {
	destroyerr := v.Destroy()
	if destroyerr != nil {
		v.t.Fatalf("%s could not be destroyed", v, destroyerr)
	}
}

func TestProgram(t *testing.T) {
	v := i(t)
	defer d(v)

	v.Run("invoking program without arguments",
		nil, nil,
	).Must(
		Print(""), PrintErr(USAGE), ExitWith(Usage),
	)

	v.Run("invoking program with tracing",
		[]string{"-T"}, nil,
	).Must(
		Print(""),
		PrintErr("error: the file /.trace must exist to enable tracing\n"),
		ExitWith(PermissionDenied),
	)

	v.Run("invoking program with tracing as unprivileged user",
		[]string{"-T"}, nil, Unprivileged,
	).Must(
		Print(""),
		PrintErr("error: the file /.trace must exist to enable tracing\n"),
		ExitWith(PermissionDenied),
	)

	v.Run("invoking program with add but no parameter",
		[]string{"-a"}, nil,
	).Must(
		ExitWithUsage()...,
	)

	v.Run("invoking program with delete but no parameter",
		[]string{"-d"}, nil,
	).Must(
		ExitWithUsage()...,
	)

	v.Modify("creating some files",
		D("somedirectory"),
		F("somefile"),
		F("somefile2", 1000, 1001, 0644),
		F("somedirectory/someshit", 1000, 1001, 0700),
	)

	v.Run("taking ownership of somefile2 as root",
		nil, []string{"somefile2"},
	).Must(
		SucceedQuietly()...,
	).Causes(
		Stat("somefile2", 0, 1001, 0644),
	)

	v.Run("taking ownership of somedirectory recursively",
		[]string{"-r"}, []string{"somedirectory"},
	).Must(
		SucceedQuietly()...,
	).Causes(
		Stat("somedirectory/someshit", 0, 1001),
	)

	v.Run("taking ownership of somefile2 as nobody",
		nil, []string{"somefile2"}, Unprivileged,
	).Must(
		Print(""),
		PrintErr("error taking ownership of somefile2: cannot take ownership of somefile2: permission denied"),
		ExitWith(PermissionDenied),
	).Causes(
		Stat("somefile2", 0, 1001, 0644),
	)

	v.Run("grant delegation on somefile2 as nobody",
		[]string{"-a", v.unprivilegedUser}, []string{"somefile2"}, Unprivileged,
	).Must(
		Print(""),
		PrintErr("error: adding delegations is a privileged operation"),
		ExitWith(PermissionDenied),
	)

	v.Run("grant delegation on somefile2 as root",
		[]string{"-a", v.unprivilegedUser}, []string{"somefile2"},
	).Must(
		SucceedQuietly()...,
	).Causes(
		Stat(".takeown.delegations", 0, 0, 0600),
	)

	v.Run("list delegations as root",
		[]string{"-l"}, []string{"."},
	).Must(
		Print("nobody:	%s/somefile2", v.Datadir()),
		PrintErr(""),
		Succeed(),
	).Causes(
		Stat(".takeown.delegations", 0, 0, 0600),
	)

	v.Run("taking ownership of somefile2 as nobody after delegation",
		nil, []string{"somefile2"}, Unprivileged,
	).Must(
		SucceedQuietly()...
	).Causes(
		Stat("somefile2", v.unprivilegedUid, 1001, 0644),
	)
}
