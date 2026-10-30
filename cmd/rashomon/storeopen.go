package main

import (
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// openStoreForRead opens the store without creating one, and reports
// ErrNoStore when there is nothing here.
//
// Every command that only reads or evicts uses this. Creating a store to answer
// a question mints an install identity and a per-install HMAC key, so a user
// who ran `report` or `forget` once would afterwards be told by `status` that a
// store is present -- true, and only because a different command fabricated it.
func openStoreForRead() (*store.Store, error) {
	root, err := store.DefaultRoot()
	if err != nil {
		return nil, err
	}
	return store.OpenExisting(root)
}

// nothingRecordedHere is what an evicting command says when there is no store.
// Not an error: asking to forget something on a machine that has recorded
// nothing is a satisfied request, and exiting non-zero would tell a user their
// privacy action failed when it had nothing to do.
const nothingRecordedHere = "nothing has been recorded here, so there is nothing to forget"

func openStore() (*store.Store, error) {
	root, err := store.DefaultRoot()
	if err != nil {
		return nil, err
	}
	return store.Open(root)
}

// storeExistsAt asks whether a root holds a store without opening one.
// checkPaused, cmdPause and cmdResume all need to ask this about a root
// before -- or instead of -- ever opening a *Store: opening CREATES a store,
// and none of the three may do that as a side effect of a question about
// recording state.
func storeExistsAt(root string) bool {
	_, err := os.Stat(filepath.Join(root, installMetaFile))
	return err == nil
}
