package launch

import (
	"time"

	"github.com/ctfer-io/chall-manager/pkg/state"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func untilFromNow(reqDates any) *time.Time {
	// No date = infinite resources
	if reqDates == nil {
		return nil
	}

	switch d := (reqDates).(type) {
	case *LaunchRequest_Timeout:
		now := time.Now()
		until := now.Add(d.Timeout.AsDuration())
		return &until

	case *LaunchRequest_Until:
		until := d.Until.AsTime()
		return &until

	default:
		panic("can't handle this case")
	}
}

func response(st *state.State) *LaunchResponse {
	res := &LaunchResponse{
		ConnectionInfo: st.Outputs.ConnectionInfo,
		Flag:           st.Outputs.Flag,
	}
	if st.Metadata.Until != nil {
		res.Until = timestamppb.New(*st.Metadata.Until)
	}
	return res
}
