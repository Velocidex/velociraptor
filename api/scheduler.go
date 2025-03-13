package api

import (
	"fmt"
	"io"
	"strings"
	"sync"

	errors "github.com/go-errors/errors"
	"google.golang.org/grpc/peer"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *ApiServer) Scheduler(
	stream api_proto.API_SchedulerServer) error {

	defer Instrument("Scheduler")()

	users := services.GetUserManager()
	ctx := stream.Context()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return Status(self.verbose, err)
	}

	// This is usually only for minions
	permissions := acls.DATASTORE_ACCESS
	peer_name := user_record.Name
	perm, err := services.CheckAccess(org_config_obj, peer_name, permissions)
	if !perm || err != nil {
		return PermissionDenied(err,
			fmt.Sprintf("User %v is not allowed to read notebooks.", peer_name))
	}

	// Update the peer name to make it unique
	peer_addr, ok := peer.FromContext(ctx)
	if ok {
		peer_name = strings.Split(peer_addr.Addr.String(), ":")[0]
	}

	scheduler, err := services.GetSchedulerService(org_config_obj)
	if err != nil {
		return Status(self.verbose, err)
	}

	req, err := stream.Recv()
	if err == io.EOF {
		return nil
	}

	if err != nil {
		return Status(self.verbose, err)
	}

	if req.Queue == "" || req.Type != "register" {
		return errors.New("First request must be a register request")
	}

	job_chan, err := scheduler.RegisterWorker(ctx, req.Queue,
		peer_name, int(req.Priority))
	if err != nil {
		return Status(self.verbose, err)
	}

	var mu sync.Mutex
	in_flight := make(map[uint64]services.SchedulerJob)
	defer func() {
		mu.Lock()
		defer mu.Unlock()

		// There should be no jobs in flight unless the client
		// suddenly disconnects
		for _, j := range in_flight {
			j.Done("", errors.New("Disconnected"))
		}
	}()

	// Watch for resoonses and close off any outstanding ones.
	go func() {
		for {
			req, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)
				logger.Error("Scheduler: <red>%v</>", err)
				return
			}

			if req.Type == "response" {
				mu.Lock()
				in_flight_job, pres := in_flight[req.Id]
				if pres {
					var err error
					if req.Error != "" {
						err = errors.New(req.Error)
					}
					in_flight_job.Done(req.Response, err)
					delete(in_flight, req.Id)
				}
				mu.Unlock()
			}

		}
	}()

	// Spin forever waiting on jobs
	for {
		select {
		case <-ctx.Done():
			return nil

		case job_req, ok := <-job_chan:
			if !ok {
				return nil
			}

			// Hold onto the job until response comes.
			id := utils.GetId()
			mu.Lock()
			in_flight[id] = job_req
			mu.Unlock()

			err := stream.Send(&api_proto.ScheduleResponse{
				Id:    id,
				Queue: job_req.Queue,
				Job:   job_req.Job,
				OrgId: job_req.OrgId,
			})
			if err != nil {
				return Status(self.verbose, err)
			}
		}
	}
}
