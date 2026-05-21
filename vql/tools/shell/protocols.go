package shell

import (
	"context"

	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

func (self *ShellSession) IsRunning() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self._IsRunning
}

func (self *ShellSession) Query() types.StoredQuery {
	return &SessionQuery{
		session: self,
	}
}

// Set the session into the running state and return the previous
// state.
func (self *ShellSession) startRunning() bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self._IsRunning {
		return true
	}

	self._IsRunning = true
	return false
}

type SessionQuery struct {
	session *ShellSession
}

// The session is a query and can be SELECTed from. Only a single
// query can select from the session and this query controls the
// lifetime of the session - When that query exists the subprocess is
// terminated and the session is unmounted.
func (self *SessionQuery) Eval(
	ctx context.Context, scope vfilter.Scope) <-chan vfilter.Row {
	output_chan := make(chan types.Row)

	// Only one runner is allowed
	is_running := self.session.startRunning()
	if is_running {
		scope.Log("shell_session: Session %v is already running",
			self.session.Name)
		close(output_chan)
		return output_chan
	}

	return self.session.output
}

func (self *ShellSession) Close() {
	self.owner.Remove(self.Name)

	self.stdin.Close()
	self.cancel()
	self.wg.Wait()
	self.command.Wait()
	close(self.output)
}
