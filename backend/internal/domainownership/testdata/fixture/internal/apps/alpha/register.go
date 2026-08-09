package alpha

import (
	"github.com/scottzx/1Agents/backend/internal/appregistry"
	"github.com/scottzx/1Agents/backend/internal/apps/alpha/repository"
)

func init() {
	appregistry.Register(appregistry.AppManifest{ID: "alpha"})
	repository.MustOpen()
}
