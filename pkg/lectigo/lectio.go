package lectigo

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/mattismoel/lectigo/util"
	"golang.org/x/exp/maps"
	"golang.org/x/net/html"
	"google.golang.org/api/calendar/v3"
)

type LectioLoginInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
	SchoolID string `json:"schoolID"`
}

type Lectio struct {
	Context   context.Context
	Cancel    context.CancelFunc
	LoginInfo *LectioLoginInfo
}

type Module struct {
	Id           string    `json:"id"`          // The ID of the module
	Title        string    `json:"title"`       // Title of the module (eg. 3a Dansk)
	StartDate    time.Time `json:"startDate"`   // The start date of the module. This includes the date as well as the time of start (eg. 09:55)
	EndDate      time.Time `json:"endDate"`     // The end date of the module. This includes the date as well as the time of end (eg. 11:25)
	Location     string    `json:"location"`    // The room of the module (eg. 22)
	Teacher      string    `json:"teacher"`     // The teacher of the class
	Group        string    `json:"group"`       // The group assigned the class (e.g. 2a MU)
	Homework     string    `json:"homework"`    // Homework for the module
	Description  string    `json:"description"` // Notes and description by the teacher
	ModuleStatus string    `json:"status"`      // The status of the module (eg. "Ændret" or "Aflyst")
}

func NewLectio(loginInfo *LectioLoginInfo) (*Lectio, error) {
	loginUrl := fmt.Sprintf("https://www.lectio.dk/lectio/%s/login.aspx", loginInfo.SchoolID)

	ctx, cancel := chromedp.NewContext(context.Background(), chromedp.WithErrorf(log.Printf))

	loginTask := chromedp.Tasks{
		chromedp.Navigate(loginUrl),
		chromedp.WaitVisible("#username"),
		chromedp.SendKeys("#username", loginInfo.Username),
		chromedp.SendKeys("#password", loginInfo.Password),
		chromedp.Click("#m_Content_submitbtn2", chromedp.NodeVisible),
	}

	err := chromedp.Run(ctx, loginTask)
	if err != nil {
		log.Fatal(err)
	}

	lectio := &Lectio{
		Context:   ctx,
		Cancel:    cancel,
		LoginInfo: loginInfo,
	}
	return lectio, nil
}

// Converts a Lectio module to a Google Calendar event
func (m *Module) ToGoogleEvent() *GoogleEvent {
	calendarColorID := ""
	switch m.ModuleStatus {
	case "Aflyst!":
		calendarColorID = "4"
	case "Ændret!":
		calendarColorID = "2"
	}

	return &GoogleEvent{
		Id:          "lec" + m.Id,
		Description: createEventDescription(m),
		Start: &calendar.EventDateTime{
			DateTime: m.StartDate.Format(time.RFC3339),
			TimeZone: "Europe/Copenhagen",
		},
		End: &calendar.EventDateTime{
			DateTime: m.EndDate.Format(time.RFC3339),
			TimeZone: "Europe/Copenhagen",
		},
		Location: m.Location,
		Summary:  m.Title,
		ColorId:  calendarColorID,
		Status:   "confirmed",
	}
}

func (l *Lectio) GetSchedule(week int) (map[string]Module, error) {
	modules := make(map[string]Module)

	weekString := fmt.Sprintf("%v%v", week, time.Now().Year())
	scheduleUrl := fmt.Sprintf("https://www.lectio.dk/lectio/%s/SkemaNy.aspx?week=%v", l.LoginInfo.SchoolID, weekString)

	// Get schedule page by using chromedp
	var scheduleHTML string

	scheduleTask := chromedp.Tasks{
		chromedp.WaitReady("body"),
		chromedp.Navigate(scheduleUrl),
		chromedp.InnerHTML("#s_m_Content_Content_SkemaNyMedNavigation_skema_skematabel", &scheduleHTML),
	}
	err := chromedp.Run(l.Context, scheduleTask)
	if err != nil {
		return nil, err
	}

	doc, err := html.Parse(strings.NewReader(scheduleHTML))
	if err != nil {
		return nil, err
	}

	// Expressions for matching and splitting time format
	reDateMatch := regexp.MustCompile(`(\d{1,2}\/\d{1,2}-20\d{2}\s\d{2}:\d{2}\stil\s\d{2}:\d{2})`)
	reDateSplit := regexp.MustCompile(`\/|-|:+|\s+`)

	// Find all <a> elements
	var getAllModules func(n *html.Node)
	getAllModules = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" && len(n.Attr) == 4 {

			var module Module

			// Extract ID from URL of the module
			params, _ := url.ParseQuery(strings.Split(n.Attr[0].Val, "?")[1])
			module.Id = params.Get("absid")

			// Get module details/elements
			moduleElements := strings.Split(n.Attr[3].Val, "\n")

			// Loop over all elements of current module
			for i := 0; i != len(moduleElements); i++ {

				if reDateMatch.Match([]byte(moduleElements[i])) {
					// Check element for assigned time and date
					module.StartDate, module.EndDate, err = util.ParseTimeAndDate(moduleElements[i], reDateSplit)

					if err != nil {
						fmt.Println("Failed to parse date and time")
						return
					}

				} else if moduleElements[i] == "Ændret!" || moduleElements[i] == "Aflyst!" {
					// Check for status on module
					module.ModuleStatus = moduleElements[i]

				} else if strings.HasPrefix(moduleElements[i], "Lærere: ") || strings.HasPrefix(moduleElements[i], "Lærer: ") {
					// Check for assigned teachers
					module.Teacher = moduleElements[i]

				} else if strings.HasPrefix(moduleElements[i], "Lokale: ") || strings.HasPrefix(moduleElements[i], "Lokaler: ") {
					// Check for assigned location
					module.Location = moduleElements[i]

				} else if strings.HasPrefix(moduleElements[i], "Hold: ") {
					// Check for group assigned to lesson
					module.Group = strings.TrimPrefix(moduleElements[i], "Hold: ")

				} else if moduleElements[i] == "Lektier:" {
					// Check for homework for the lesson

					for j := i + 1; j != len(moduleElements); j++ {
						if !strings.HasPrefix(moduleElements[j], "Note:") {
							module.Homework += moduleElements[j] + "\n"
							i = j
						} else {
							break
						}
					}

				} else if moduleElements[i] == "Note:" {
					// Check for description and notes of the lesson
					for j := i + 1; j != len(moduleElements); j++ {
						module.Description += moduleElements[j] + "\n"
						i = j
					}

				} else if moduleElements[i] != "" {
					// Assign as title if no other match
					module.Title = moduleElements[i] + " - "
				}
			}
			module.Title += module.Group
			modules[module.Id] = module
		}

		// Loop to next module until week is done
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			getAllModules(c)
		}
	}
	getAllModules(doc)
	return modules, nil
}

// Gets the Lectio schedule from the current weeks and weekCount weeks ahead.
func (l *Lectio) GetScheduleWeeks(weekCount int) (modules map[string]Module, err error) {
	modules = make(map[string]Module)
	_, week := time.Now().ISOWeek()

	for i := 0; i < weekCount; i++ {
		weekModules, err := l.GetSchedule(week + i)
		if err != nil {
			return nil, err
		}
		maps.Copy(modules, weekModules)
	}

	return modules, nil
}

// Checks if two Lectio modules are equal
func (m1 *Module) Equals(m2 *Module) bool {
	b := m1.Id == m2.Id &&
		m1.StartDate.Equal(m2.StartDate) &&
		m1.EndDate.Equal(m2.EndDate) &&
		m1.ModuleStatus == m2.ModuleStatus &&
		m1.Location == m2.Location &&
		createEventDescription(m1) == m2.Description
	return b
}

// Converts input Lectio modules to a JSON object at the specified path
func ModulesToJSON(modules map[string]Module, filename string) error {
	filename, _ = strings.CutSuffix(filename, ".json")
	b, err := json.Marshal(modules)
	if err != nil {
		return err
	}

	err = os.WriteFile(fmt.Sprintf("%s.json", filename), b, 0644)
	if err != nil {
		return err
	}

	return nil
}

func createEventDescription(m *Module) string {
	description := m.Teacher + "\n"
	if m.Description != "" {
		description += fmt.Sprintf("Noter: %s", m.Description)
	}
	if m.Homework != "" {
		description += fmt.Sprintf("Lektier:\n%s", m.Homework)
	}
	return description
}
