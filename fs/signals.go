// Copyright (c) 2021 MinIO, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package minfs

import (
	"os"
	"os/signal"
)

// signalTrap traps the registered signals and notifies the caller.
func signalTrap(sig ...os.Signal) <-chan bool {
	// channel to notify the caller.
	trapCh := make(chan bool, 1)

	go func(chan<- bool) {
		// channel to receive signals.
		sigCh := make(chan os.Signal, 1)
		defer close(sigCh)

		// `signal.Notify` registers the given channel to
		// receive notifications of the specified signals.
		signal.Notify(sigCh, sig...)

		// Wait for the signal.
		<-sigCh

		// Once signal has been received stop signal Notify handler.
		signal.Stop(sigCh)

		// Notify the caller.
		trapCh <- true
	}(trapCh)

	return trapCh
}
