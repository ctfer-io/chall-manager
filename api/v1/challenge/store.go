package challenge

func NewStore() *Store {
	return &Store{}
}

// Store holds a distributed filesystem along its integrity to handle
// challenge scenario storage.
// In case of updates, it locks the instances and update them to avoid
// infrastructure drift through time.
//
// The etcd key is "store".
// The filesystem to write into is "<storage>/challenge/<id>".
type Store struct {
	UnimplementedChallengeStoreServer
}
