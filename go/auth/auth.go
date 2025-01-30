// Copyright (c) 2023 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"context"

	"go.uber.org/fx"
)

// Action performed by user
type Action string

const (
	Create           Action = "Create"
	Get                     = "Get"
	Update                  = "Update"
	Delete                  = "Delete"
	DeleteCollection        = "DeleteCollection"
	List                    = "List"
)

// Auth interface for auth
type Auth interface {
	UserAuthorized(context.Context, string, Action, string) (bool, error)
	UserAuthenticated(context.Context) (bool, error)
}

type DummyAuth struct{}

func (_ DummyAuth) UserAuthorized(context.Context, string, Action, string) (bool, error) {
	return true, nil
}

func (_ DummyAuth) UserAuthenticated(context.Context) (bool, error) {
	return true, nil
}

var DummyAuthModule = fx.Options(
	fx.Provide(func() Auth {
		return DummyAuth{}
	}),
)
