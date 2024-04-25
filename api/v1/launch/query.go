package launch

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"github.com/ctfer-io/chall-manager/pkg/state"
	"go.uber.org/multierr"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (server *launcherServer) QueryLaunches(_ *emptypb.Empty, stream Launcher_QueryLaunchesServer) error {
	ctx := stream.Context()

	// Trigger Top-Of-The-World lock for Query to take a snapshot of current states
	// then lock each individually, read the state and stream it.
	totw, err := TOTWLock(ctx)
	if err != nil {
		return err
	}
	defer lclose(totw)
	if err := totw.RWLock(); err != nil {
		return err
	}

	// Read all state files from filesystem rather than testing all combinations
	// of <challenge_id>/<source_id>.
	files, err := os.ReadDir(filepath.Join(global.Conf.Directory, "states"))
	if err != nil {
		if err := totw.RWUnlock(); err != nil {
			return err
		}
		return err
	}
	errs := make(chan error)
	relocked := &sync.WaitGroup{} // track goroutines that overlocked an identity
	relocked.Add(len(files))
	wg := &sync.WaitGroup{} // track goroutines that ended dealing with the identities states
	wg.Add(len(files))
	for _, f := range files {
		go func(stream Launcher_QueryLaunchesServer, relocked, wg *sync.WaitGroup, errs chan<- error, id string) {
			// Acquire lock for this identity and signal it to TOTW lock
			rw, err := lock.NewRWLock(id)
			if err != nil {
				errs <- err
				return
			}
			defer lclose(rw)
			if err := rw.RLock(); err != nil {
				errs <- err
				return
			}
			relocked.Done()

			// Stream state
			state, err := state.Load(id)
			if err != nil {
				errs <- err
				// if state loading failed, make sure to unlock before stopping else
				// it would starve other/future identity locks
				if err := rw.RUnlock(); err != nil {
					errs <- err
				}
				return
			}
			if err := stream.Send(&QueryLaunchResponse{
				ChallengeId:    state.Metadata.ChallengeId,
				SourceId:       state.Metadata.SourceId,
				LaunchResponse: response(state),
			}); err != nil {
				errs <- err
			}

			// Free lock for this identity and signal it to source goroutine
			if err := rw.RUnlock(); err != nil {
				errs <- err
			}
			wg.Done()
		}(stream, relocked, wg, errs, f.Name())
	}

	relocked.Wait()
	if err := totw.RWUnlock(); err != nil {
		return err
	}

	wg.Wait()
	close(errs)
	var merr error
	for err := range errs {
		merr = multierr.Append(merr, err)
	}
	if merr != nil {
		return err
	}
	return merr
}
