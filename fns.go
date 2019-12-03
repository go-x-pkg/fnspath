package fnspath

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
)

const (
	RemoveAttempts int = 20
	MvAttempts     int = 20
)

func IsExists(path string) (bool, error) {
	if _, e := os.Stat(path); e != nil {
		if os.IsNotExist(e) {
			return false, nil
		}
		return true, e
	}

	return true, nil
}

func MkdirAll(path string, perm os.FileMode) error {
	// Fast path: if we can tell whether path is a directory or file, stop with success or error.
	dir, err := os.Stat(path)
	if err == nil {
		if dir.IsDir() {
			return nil
		}

		return &os.PathError{"mkdir", path, syscall.ENOTDIR}
	}

	// Slow path: make sure parent exists and then call Mkdir for path.
	i := len(path)
	for i > 0 && os.IsPathSeparator(path[i-1]) { // Skip trailing path separator.
		i--
	}

	j := i
	for j > 0 && !os.IsPathSeparator(path[j-1]) { // Scan backward over element.
		j--
	}

	if j > 1 {
		// Create parent
		err = MkdirAll(path[0:j-1], perm)
		if err != nil {
			return err
		}
	}

	// Parent now exists; invoke Mkdir and use its result.
	err = os.Mkdir(path, perm)
	if err != nil {
		// Handle arguments like "foo/." by
		// double-checking that directory doesn't exist.
		dir, err1 := os.Lstat(path)
		if err1 == nil && dir.IsDir() {
			return nil
		}

		return err
	}

	err = os.Chmod(path, perm)
	if err != nil {
		return err
	}

	return nil
}

func Ensure(path string, perm os.FileMode) error {
	if ok, e := IsExists(path); e != nil {
		return e
	} else if !ok {
		if e = MkdirAll(path, perm); e != nil {
			return e
		}
	}

	return nil
}

func Remove(path string) (e error) {
	attempt := RemoveAttempts

	for attempt > 0 {
		e = os.RemoveAll(path)
		if e != nil {
			attempt--
			continue
		}

		return nil
	}

	return e
}

func MV(oldpath, newpath string) (e error) {
	attempt := MvAttempts

	for attempt > 0 {
		e = os.Rename(oldpath, newpath)
		if e != nil {
			attempt--
			continue
		}

		return nil
	}

	return e
}

func IsDirEmpty(name string) (bool, error) {
	f, e := os.Open(name)
	if e != nil {
		return false, e
	}
	defer f.Close()

	// read in ONLY one file
	_, e = f.Readdir(1)

	// and if the file is EOF... well, the dir is empty.
	if e == io.EOF {
		return true, nil
	}

	return false, e
}

func Absolutize(paths []*string) error {
	for _, p := range paths {
		v, err := filepath.Abs(*p)
		if err != nil {
			return err
		}

		*p = v
	}

	return nil
}

type pathAndMode struct {
	p    string
	mode os.FileMode
}

type PathAndModes []pathAndMode

func (pam *PathAndModes) Append(path string, mode os.FileMode) {
	*pam = append(*pam, pathAndMode{path, mode})
}

func NewPathAndModes() PathAndModes { return PathAndModes{} }

func EnsureMany(paths PathAndModes) error {
	for _, r := range paths {
		if _, e := os.Stat(r.p); os.IsNotExist(e) {
			if e = os.MkdirAll(r.p, r.mode); e != nil {
				return e
			}
		} else if e != nil {
			return e
		}
	}

	return nil
}

func AbsentMany(paths []string) (err error) {
	for _, path := range paths {
		if e := Remove(path); e != nil {
			err = e
		}
	}

	return
}

func Clear(path string) (err error) {
	if ok, err := IsExists(path); err != nil {
		return err
	} else if !ok {
		return nil
	}

	fs, err := ioutil.ReadDir(path)
	if err != nil {
		return err
	}

	for _, f := range fs {
		err = Remove(filepath.Join(path, f.Name()))
	}

	return err
}

func ToFile(path string, mode os.FileMode, reader io.Reader) error {
	dir := filepath.Dir(path)
	if e := Ensure(dir, mode); e != nil {
		return e
	}

	f, e := os.Create(path)
	if e != nil {
		return e
	}
	defer f.Close()

	if _, e := io.Copy(f, reader); e != nil {
		return e
	}

	return nil
}

func WriteFile(path string, dirMode, fileMode os.FileMode, src io.Reader) (int64, error) {
	dir := filepath.Dir(path)
	if e := Ensure(dir, dirMode); e != nil {
		return 0, e
	}

	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileMode)
	if e != nil {
		return 0, e
	}
	defer f.Close()

	sz, e := io.Copy(f, src)
	if e != nil {
		return 0, e
	}

	return sz, nil
}

func Rename(oldpath, newpath string, mode os.FileMode) error {
	if e := Ensure(filepath.Dir(newpath), mode); e != nil {
		return nil
	}

	return os.Rename(oldpath, newpath)
}

func CopyFile(source string, dest string, mode os.FileMode) error {
	sourcefile, err := os.Open(source)
	if err != nil {
		return err
	}

	defer sourcefile.Close()

	destfile, err := os.Create(dest)
	if err != nil {
		return err
	}

	defer destfile.Close()

	if _, err := io.Copy(destfile, sourcefile); err == nil {
		if mode != 0 { // custom mode
			if err = os.Chmod(dest, mode); err != nil {
				return err
			}
		} else { // copy mode from source file
			sourceinfo, err := os.Stat(source)
			if err != nil {
				return err
			}
			if err = os.Chmod(dest, sourceinfo.Mode()); err != nil {
				return err
			}
		}
	}

	return nil
}

func PathCopyDir(source string, dest string) (err error) {
	// get properties of source dir
	sourceinfo, err := os.Stat(source)
	if err != nil {
		return err
	}

	// create dest dir

	err = os.MkdirAll(dest, sourceinfo.Mode())
	if err != nil {
		return err
	}

	directory, _ := os.Open(source)

	objects, err := directory.Readdir(-1)

	for _, obj := range objects {
		sourcefilepointer := source + "/" + obj.Name()

		destinationfilepointer := dest + "/" + obj.Name()

		if obj.IsDir() {
			// create sub-directories - recursively
			err = PathCopyDir(sourcefilepointer, destinationfilepointer)
			if err != nil {
				fmt.Println(err)
			}
		} else {
			// perform copy
			err = CopyFile(sourcefilepointer, destinationfilepointer, 0)
			if err != nil {
				fmt.Println(err)
			}
		}
	}

	return
}

func CopyFileEnsureDir(src string, dst string, fileMode os.FileMode, dirMode os.FileMode) error {
	dirPath := filepath.Dir(dst)
	if e := Ensure(dirPath, dirMode); e != nil {
		return e
	}

	if e := CopyFile(src, dst, fileMode); e != nil {
		return e
	}

	return nil
}

func RemoveFileOKEvenIfNotExists(path string) (e error) {
	if e := os.Remove(path); e != nil {
		if pathErr, ok := e.(*os.PathError); ok {
			if pathErr.Err.Error() == "no such file or directory" {
				return nil
			}

			return e
		}
	}

	return nil
}
