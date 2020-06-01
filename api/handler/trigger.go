package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	"github.com/moira-alert/moira"

	"github.com/moira-alert/moira/api"
	"github.com/moira-alert/moira/api/controller"
	"github.com/moira-alert/moira/api/dto"
	"github.com/moira-alert/moira/api/middleware"
)

func trigger(router chi.Router) {
	router.Use(middleware.TriggerContext)
	router.Put("/", updateTrigger)
	router.With(middleware.TriggerContext, middleware.Populate(false)).Get("/", getTrigger)
	router.Delete("/", removeTrigger)
	router.Get("/state", getTriggerState)
	router.Route("/throttling", func(router chi.Router) {
		router.Get("/", getTriggerThrottling)
		router.Delete("/", deleteThrottling)
	})
	router.Route("/metrics", triggerMetrics)
	router.Put("/setMaintenance", setTriggerMaintenance)
	router.With(middleware.DateRange("-1hour", "now")).With(middleware.TargetName("t1")).Get("/render", renderTrigger)
}

func updateTrigger(writer http.ResponseWriter, request *http.Request) {
	triggerID := middleware.GetTriggerID(request)

	trigger, err := getTriggerFromRequest(request)
	if err != nil {
		render.Render(writer, request, err)
		return
	}

	if trigger.Desc != nil {
		triggerData := moira.TriggerData{Desc: *trigger.Desc, Name: trigger.Name}
		if _, err := triggerData.GetPopulatedDescription(moira.NotificationEvents{}); err != nil {
			render.Render(writer, request, api.ErrorRender(
				fmt.Errorf("you have an error in your Go template: %v", err)))
			return
		}
	}

	timeSeriesNames := middleware.GetTimeSeriesNames(request)
	response, err := controller.UpdateTrigger(database, &trigger.TriggerModel, triggerID, timeSeriesNames)
	if err != nil {
		render.Render(writer, request, err)
		return
	}

	if err := render.Render(writer, request, response); err != nil {
		render.Render(writer, request, api.ErrorRender(err))
		return
	}
}

func removeTrigger(writer http.ResponseWriter, request *http.Request) {
	triggerID := middleware.GetTriggerID(request)
	err := controller.RemoveTrigger(database, triggerID)
	if err != nil {
		render.Render(writer, request, err)
	}
}

func getTrigger(writer http.ResponseWriter, request *http.Request) {
	triggerID := middleware.GetTriggerID(request)
	if triggerID == "testlog" {
		panic("Test for multi line logs")
	}
	trigger, err := controller.GetTrigger(database, triggerID)
	if err != nil {
		render.Render(writer, request, err)
		return
	}

	if needToPopulate := middleware.GetPopulated(request); needToPopulate && trigger.Desc != nil {
		triggerData := moira.TriggerData{Desc: *trigger.Desc, Name: trigger.Name}

		eventsList, err := controller.GetTriggerEvents(database, triggerID, 0, 3)
		if err != nil {
			render.Render(writer, request, err)
		}

		*trigger.Desc, _ = triggerData.GetPopulatedDescription(eventsList.List)
	}

	if err := render.Render(writer, request, trigger); err != nil {
		render.Render(writer, request, api.ErrorRender(err))
	}
}

func getTriggerState(writer http.ResponseWriter, request *http.Request) {
	triggerID := middleware.GetTriggerID(request)
	triggerState, err := controller.GetTriggerLastCheck(database, triggerID)
	if err != nil {
		render.Render(writer, request, err)
		return
	}
	if err := render.Render(writer, request, triggerState); err != nil {
		render.Render(writer, request, api.ErrorRender(err))
	}
}

func getTriggerThrottling(writer http.ResponseWriter, request *http.Request) {
	triggerID := middleware.GetTriggerID(request)
	triggerState, err := controller.GetTriggerThrottling(database, triggerID)
	if err != nil {
		render.Render(writer, request, err)
		return
	}
	if err := render.Render(writer, request, triggerState); err != nil {
		render.Render(writer, request, api.ErrorRender(err))
	}
}

func deleteThrottling(writer http.ResponseWriter, request *http.Request) {
	triggerID := middleware.GetTriggerID(request)
	err := controller.DeleteTriggerThrottling(database, triggerID)
	if err != nil {
		render.Render(writer, request, err)
	}
}

func setTriggerMaintenance(writer http.ResponseWriter, request *http.Request) {
	triggerID := middleware.GetTriggerID(request)
	triggerMaintenance := dto.TriggerMaintenance{}
	if err := render.Bind(request, &triggerMaintenance); err != nil {
		render.Render(writer, request, api.ErrorInvalidRequest(err))
		return
	}
	userLogin := middleware.GetLogin(request)
	timeCallMaintenance := time.Now().Unix()

	err := controller.SetTriggerMaintenance(database, triggerID, triggerMaintenance, userLogin, timeCallMaintenance)
	if err != nil {
		render.Render(writer, request, err)
	}
}
