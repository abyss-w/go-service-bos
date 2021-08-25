package bos

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/baidubce/bce-sdk-go/bce"
	"github.com/baidubce/bce-sdk-go/services/bos/api"

	ps "github.com/beyondstorage/go-storage/v4/pairs"
	"github.com/beyondstorage/go-storage/v4/pkg/iowrap"
	"github.com/beyondstorage/go-storage/v4/services"
	. "github.com/beyondstorage/go-storage/v4/types"
)

func (s *Storage) create(path string, opt pairStorageCreate) (o *Object) {
	rp := s.getAbsPath(path)

	if opt.HasObjectMode && opt.ObjectMode.IsDir() {
		if !s.features.VirtualDir {
			return
		}

		rp += "/"
		o = s.newObject(true)
		o.Mode |= ModeDir
	} else {
		o = s.newObject(false)
		o.Mode |= ModeRead
	}
	o.ID = rp
	o.Path = path

	return o
}

func (s *Storage) delete(ctx context.Context, path string, opt pairStorageDelete) (err error) {
	rp := s.getAbsPath(path)

	if opt.HasObjectMode && opt.ObjectMode.IsDir() {
		if !s.features.VirtualDir {
			err = services.PairUnsupportedError{Pair: ps.WithObjectMode(opt.ObjectMode)}
			return
		}
		rp += "/"
	}

	err = s.client.DeleteObject(s.bucket, rp)
	if err != nil {
		return err
	}

	return nil
}

func (s *Storage) list(ctx context.Context, path string, opt pairStorageList) (oi *ObjectIterator, err error) {
	panic("not implemented")
}

func (s *Storage) metadata(opt pairStorageMetadata) (meta *StorageMeta) {
	meta = NewStorageMeta()
	meta.Name = s.bucket
	meta.WorkDir = s.workDir
	return meta
}

func (s *Storage) read(ctx context.Context, path string, w io.Writer, opt pairStorageRead) (n int64, err error) {
	rp := s.getAbsPath(path)

	output, err := s.client.GetObject(s.bucket, rp, nil)
	if err != nil {
		return 0, err
	}

	rc := output.Body
	if opt.HasIoCallback {
		rc = iowrap.CallbackReadCloser(rc, opt.IoCallback)
	}

	return io.Copy(w, rc)
}

func (s *Storage) stat(ctx context.Context, path string, opt pairStorageStat) (o *Object, err error) {
	rp := s.getAbsPath(path)

	if opt.HasObjectMode && opt.ObjectMode.IsDir() {
		if !s.features.VirtualDir {
			err = services.PairUnsupportedError{Pair: ps.WithObjectMode(opt.ObjectMode)}
			return nil, err
		}

		rp += "/"
	}

	output, err := s.client.GetObject(s.bucket, rp, nil)
	if err != nil {
		return nil, err
	}

	o = s.newObject(true)
	o.ID = rp
	o.Path = path

	if opt.HasObjectMode && opt.ObjectMode.IsDir() {
		o.Mode |= ModeDir
	} else {
		o.Mode |= ModeRead
	}

	o.SetContentLength(output.ContentLength)
	lastModified, err := time.Parse(time.RFC1123, output.LastModified)
	if err != nil {
		return nil, err
	}
	o.SetLastModified(lastModified)

	if output.ContentType != "" {
		o.SetContentType(output.ContentType)
	}
	if output.ETag != "" {
		o.SetEtag(output.ETag)
	}

	var sm ObjectSystemMetadata
	if v := output.StorageClass; v != "" {
		sm.StorageClass = v
	}

	o.SetSystemMetadata(sm)

	return
}

func (s *Storage) write(ctx context.Context, path string, r io.Reader, size int64, opt pairStorageWrite) (n int64, err error) {
	if size > writeSizeMaximum {
		err = fmt.Errorf("size limit exceeded: %w", services.ErrRestrictionDissatisfied)
		return 0, err
	}

	rp := s.getAbsPath(path)

	if opt.HasIoCallback {
		r = iowrap.CallbackReader(r, opt.IoCallback)
	}

	body, err := bce.NewBodyFromSizedReader(r, size)
	if err != nil {
		return 0, err
	}
	putArgs := &api.PutObjectArgs{
		ContentLength: size,
	}

	if opt.HasContentMd5 {
		putArgs.ContentMD5 = opt.ContentMd5
	}
	if opt.HasStorageClass {
		putArgs.StorageClass = opt.StorageClass
	}

	_, err = s.client.PutObject(s.bucket, rp, body, putArgs)
	if err != nil {
		return 0, err
	}

	return size, nil
}
