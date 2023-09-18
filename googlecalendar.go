package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

type CalendarInfo struct {
	CalendarID string `json:"calendarID"`
}

type GoogleCalendar struct {
	Client       *http.Client
	Service      *calendar.Service
	CalendarInfo *CalendarInfo
}

func NewGoogleCalendar(CalendarInfo *CalendarInfo) *GoogleCalendar {
	ctx := context.Background()
	bytes, err := os.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Could not read client secret file: %v", err)
	}

	config, err := google.ConfigFromJSON(bytes, calendar.CalendarScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := *GetClient(config)
	service, err := calendar.NewService(ctx, option.WithHTTPClient(&client))
	if err != nil {
		log.Fatalf("Could not get Calendar client: %v", err)
	}

	return &GoogleCalendar{
		Client:       &client,
		Service:      service,
		CalendarInfo: CalendarInfo,
	}
}

func GetClient(config *oauth2.Config) *http.Client {
	tokenFile := "token.json"
	token, err := tokenFromFile(tokenFile)
	if err != nil {
		token = getTokenFromWeb(config)
		saveToken(tokenFile, token)
	}
	return config.Client(context.Background(), token)
}

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOnline)

	fmt.Printf("Go to the following link in your browser and type the authorization code %q\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Could not read authorization code: %v", err)
	}

	token, err := config.Exchange(context.TODO(), authCode)

	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}

	return token

}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	token := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(token)
	return token, err
}

func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)

	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func (c *GoogleCalendar) AddModules(modules map[string]Module) {
	startTime := time.Now()
	// log.Printf("MISSING MODULE COUNT %v\n", len(modules))
	for key, module := range modules {
		start := &calendar.EventDateTime{DateTime: module.StartDate.Format(time.RFC3339), TimeZone: "Europe/Copenhagen"}
		end := &calendar.EventDateTime{DateTime: module.EndDate.Format(time.RFC3339), TimeZone: "Europe/Copenhagen"}

		//Find color ID depending on the status of the module. "aflyst" results in red, "ændret" results in green
		calendarColorID := ""
		switch module.ModuleStatus {
		case "aflyst":
			calendarColorID = "4"
		case "ændret":
			calendarColorID = "2"
		}
		calEvent := &calendar.Event{
			Id:          "lec" + key,
			Start:       start,
			End:         end,
			ColorId:     calendarColorID,
			Summary:     module.Title,
			Description: module.Teacher,
			Location:    module.Room,
			Status:      "confirmed",
		}
		// log.Printf("FROM ADD MODULES\n %q\n%v\n", calEvent.Id, PrettyPrint(calEvent))
		fmt.Println("ID", calEvent.Id)
		// If event already exists in Google Calendar
		if event, err := c.Service.Events.Get(googleCalendarConfig.CalendarID, calEvent.Id).Do(); err == nil {
			log.Printf("Event %q already exists.\n", key)
			if event.Status == "cancelled" {
				log.Printf("Found deleted event %q\n", key)
				_, err := c.Service.Events.Update(googleCalendarConfig.CalendarID, calEvent.Id, calEvent).Do()
				if err != nil {
					log.Fatalf("Could not update deleted event: %v\n", err)
				}
				continue
			}
		} else {
			_, err := c.Service.Events.Insert(googleCalendarConfig.CalendarID, calEvent).Do()
			if err != nil {
				log.Fatalf("Could not insert event %q: %v\n", calEvent.Id, err)
			}
		}
	}
	log.Printf("Added modules to Google Calendar in %v", time.Since(startTime))

}

// Returns all modules from Google Calendar
func (c *GoogleCalendar) GetModules(weekCount int) map[string]Module {
	startDate := RoundDateToDay(GetMonday())
	endDate := RoundDateToDay(startDate.AddDate(0, 0, 7))
	events, err := c.Service.Events.List(c.CalendarInfo.CalendarID).ShowDeleted(true).TimeMin(startDate.Format(time.RFC3339)).TimeMax(endDate.Format(time.RFC3339)).Do()
	if err != nil {
		log.Fatalf("Could not list the events of the calendar with ID %q: %v\n", c.CalendarInfo.CalendarID, err)
	}
	for _, event := range events.Items {
		// If Google Calendar event is not a lectio module
		if !strings.Contains(event.Id, "lec") {
			continue
		}
		// fmt.Printf("MODULE:\n%v\n", PrettyPrint(event))
	}

	googleCalModules := make(map[string]Module)

	if err != nil {
		log.Fatalf("Could not load location: %v\n", err)
	}

	// _, currWeek := time.Now().ISOWeek()
	for _, event := range events.Items {
		// fmt.Println("GOOGE MODULE", event.Id)
		startTime, err := time.Parse(time.RFC3339, event.Start.DateTime)
		if err != nil {
			// The event is an all-day event - skip
			log.Printf("%v: Could not parse the date: %v\n", event.Summary, err)
			continue
		}

		endTime, err := time.Parse(time.RFC3339, event.End.DateTime)
		if err != nil {
			log.Printf("%v: Could not parse the date: %v\n", event.Summary, err)
		}

		id := strings.TrimPrefix(event.Id, "lec")
		googleCalModules[id] = Module{
			Title:     event.Summary,
			StartDate: startTime,
			EndDate:   endTime,
			Room:      event.Location,
			Teacher:   event.Description,
			Homework:  event.Description,
		}

		// fmt.Println(PrettyPrint(event))
	}

	// fmt.Println(googleCalModules)
	return googleCalModules
}

func (*GoogleCalendar) UpdateCalendar(lectioModules map[string]Module, googleModules map[string]Module) {
	// Finds the missing and extra modules in the Google Calendar with respect to the modules in the Lectio schedule
	extras, missing := CompareMaps(lectioModules, googleModules)

	// fmt.Println("LECTIO MODULES ---------------------------------")
	// for key, _ := range lectioModules {
	// 	fmt.Printf("KEY: %q\n", key)
	// }
	// fmt.Printf("ENTRIES: %v\n", len(lectioModules))
	// fmt.Println("------------------------------------------------\n\n")

	// fmt.Println("GOOGLE MODULES ---------------------------------")
	// for key, _ := range googleModules {
	// 	fmt.Printf("KEY: %q\n", key)
	// }
	// fmt.Printf("ENTRIES: %v\n", len(googleModules))
	// fmt.Println("------------------------------------------------")
	fmt.Println("MISSING GOOGLE MODULES ---------------------------------")
	for key, miss := range missing {
		fmt.Printf("%q\n%v\n", key, PrettyPrint(miss))
	}
	fmt.Println("--------------------------------------------------------\n\n")

	fmt.Println("EXTRA GOOGLE MODULES ---------------------------------")
	for key, extra := range extras {
		fmt.Printf("%q\n%v\n", key, PrettyPrint(extra))
	}
	fmt.Println("--------------------------------------------------------\n\n")
	// Deletes all the extra events from the Google Calendar
	// for id := range extras {
	// 	// fmt.Println("EXTRA")
	// 	if err := googleCalendar.Service.Events.Delete(googleCalendar.CalendarInfo.CalendarID, id).Do(); err != nil {
	// 		log.Fatalf("Could not delete extra event: %v\n", err)
	// 	}
	// 	log.Printf("Deleted removed event %q\n", id)
	// }

	// googleCalendar.AddModules(missing)
}

func GoogleEventToModule(event *calendar.Event) Module {
	start, err := time.Parse(time.RFC3339, event.Start.DateTime)
	if err != nil {
		log.Fatalf("Could not parse start date: %v\n", err)
	}

	end, err := time.Parse(time.RFC3339, event.End.DateTime)
	if err != nil {
		log.Fatalf("Could not parse end date: %v\n", err)
	}
	return Module{
		Title:     event.Summary,
		StartDate: start,
		EndDate:   end,
		Room:      event.Location,
		Teacher:   event.Description,
		Homework:  event.Description,
	}
}

func (c *GoogleCalendar) Clear() {
	s := time.Now()
	pageToken := ""
	eventCount := 0
	for {
		req := c.Service.Events.List(c.CalendarInfo.CalendarID).MaxResults(50)
		if pageToken != "" {
			req.PageToken(pageToken)
		}
		r, err := req.Do()
		if err != nil {
			log.Fatalf("Could not retrieve events: %v\n", err)
		}
		for _, item := range r.Items {
			if strings.Contains(item.Id, "lec") {
				err := c.Service.Events.Delete(c.CalendarInfo.CalendarID, item.Id).Do()
				if err != nil {
					log.Fatalf("Could not delete event %v: %v\n", item.Id, err)
				}
				eventCount++
				// log.Printf("Found event %q\n", item.Id)
			}
		}

		pageToken = r.NextPageToken
		if pageToken == "" {
			break
		}
	}
	log.Printf("Found and deleted %v events in %v\n", eventCount, time.Since(s))

	// events, err := c.Service.Events.List(c.CalendarInfo.CalendarID).MaxResults(1000).Do()
	// if err != nil {
	// 	log.Fatalf("Could not list events: %v\n", err)
	// }
	// fmt.Println("LENGTH: ", len(events.Items))
	// for _, event := range events.Items {
	// 	// fmt.Println(event.Id)
	// 	if strings.Contains(event.Id, "lec") {
	// 		log.Printf("Found event %q\n", event.Id)
	// 		// if err := c.Service.Events.Delete(c.CalendarInfo.CalendarID, event.Id).Do(); err != nil {
	// 		// 	log.Fatalf("Could not delete event: %v\n", err)
	// 		// }
	// 	}
	// }
}
