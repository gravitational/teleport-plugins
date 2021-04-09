/*
Copyright 2020 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

import (
	"context"
	"net/http"
	"time"

	"github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/trace"

	"github.com/julienschmidt/httprouter"
)

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request, p httprouter.Params, session *session) error {
	// Remove the session from the backend.
	err := h.c.AuthClient.DeleteAppSession(context.Background(), services.DeleteAppSessionRequest{
		SessionID: session.ws.GetName(),
	})
	if err != nil {
		return trace.Wrap(err)
	}

	// Set Max-Age to 0 to tell the browser to delete this cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		Expires:  time.Unix(0, 0),
		SameSite: http.SameSiteLaxMode,
	})
	http.Error(w, "Logged out.", http.StatusOK)

	return nil
}
