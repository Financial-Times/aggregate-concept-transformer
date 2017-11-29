package concept

import (
	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	"github.com/Financial-Times/service-status-go/gtg"
)

type HealthService struct {
	config *config
	svc    Service
	Checks []fthealth.Check
}

type config struct {
	appSystemCode string
	appName       string
	port          string
	description   string
}

func NewHealthService(svc Service, appSystemCode string, appName string, port string, description string) *HealthService {
	service := &HealthService{
		config: &config{
			appSystemCode: appSystemCode,
			appName:       appName,
			port:          port,
			description:   description,
		},
		svc: svc,
	}
	service.Checks = svc.Healthchecks()
	return service
}

func (svc *HealthService) GTG() gtg.Status {
	var checks []gtg.StatusChecker
	for _, check := range svc.Checks {
		checks = append(checks, func() gtg.Status {
			return svc.gtgCheck(check)
		})
	}
	return gtg.FailFastParallelCheck(checks)()
}

func (svc *HealthService) gtgCheck(check fthealth.Check) gtg.Status {
	if _, err := check.Checker(); err != nil {
		return gtg.Status{GoodToGo: false, Message: err.Error()}
	}
	return gtg.Status{GoodToGo: true}
}
