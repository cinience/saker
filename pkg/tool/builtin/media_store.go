package toolbuiltin

import "context"

// MediaStoreFunc persists media data to the object store and returns the
// public URL (e.g. "/media/_default/image/ab/abc...png"). Called by
// ResolveMediaPath when an object store is available in context.
type MediaStoreFunc func(ctx context.Context, data []byte, filename, mediaType string) (url string, err error)

// MediaReadFunc reads object bytes given a /media/ URL path. Returns the
// raw bytes and the content-type. Used by resolveLocalRef to load
// previously-stored media without disk access.
type MediaReadFunc func(ctx context.Context, mediaURL string) (data []byte, contentType string, err error)

type mediaStoreKey struct{}

type mediaStoreFuncs struct {
	store MediaStoreFunc
	read  MediaReadFunc
}

// WithMediaStore injects object-store callbacks into the context. Tools that
// produce or consume media check these before falling back to local disk.
func WithMediaStore(ctx context.Context, store MediaStoreFunc, read MediaReadFunc) context.Context {
	return context.WithValue(ctx, mediaStoreKey{}, mediaStoreFuncs{store: store, read: read})
}

// MediaStoreFromContext retrieves the store/read callbacks. Returns nil, nil
// when no object store has been injected (CLI mode, tests).
func MediaStoreFromContext(ctx context.Context) (MediaStoreFunc, MediaReadFunc) {
	if ctx == nil {
		return nil, nil
	}
	v, _ := ctx.Value(mediaStoreKey{}).(mediaStoreFuncs)
	return v.store, v.read
}
