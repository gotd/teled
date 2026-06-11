package rpc

import "sync"

// stagingKey identifies an in-progress upload (file ids are client-chosen, so
// they are namespaced per user).
type stagingKey struct {
	userID int64
	fileID int64
}

// stagedFile accumulates upload parts before assembly.
type stagedFile struct {
	mu    sync.Mutex
	parts map[int][]byte
	max   int
}

func (s *stagedFile) put(part int, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.parts == nil {
		s.parts = map[int][]byte{}
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	s.parts[part] = cp
	if part > s.max {
		s.max = part
	}
}

// assemble concatenates the parts in order.
func (s *stagedFile) assemble() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []byte
	for i := 0; i <= s.max; i++ {
		out = append(out, s.parts[i]...)
	}
	return out
}

// uploadStaging holds in-progress multipart uploads.
type uploadStaging struct {
	mu    sync.Mutex
	files map[stagingKey]*stagedFile
}

func newUploadStaging() *uploadStaging {
	return &uploadStaging{files: map[stagingKey]*stagedFile{}}
}

func (u *uploadStaging) file(key stagingKey) *stagedFile {
	u.mu.Lock()
	defer u.mu.Unlock()
	f, ok := u.files[key]
	if !ok {
		f = &stagedFile{}
		u.files[key] = f
	}
	return f
}

func (u *uploadStaging) take(key stagingKey) (*stagedFile, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	f, ok := u.files[key]
	if ok {
		delete(u.files, key)
	}
	return f, ok
}
