package util

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

func ConvertTimestamp(date *time.Time, stamp string) (time.Time, time.Duration, error) {
	// location, err := time.LoadLocation("Europe/Copenhagen")
	// if err != nil {
	// 	return startTime, endTime, err
	// }
	// layout := "15:04"
	// split := strings.Split(s, " til ")
	// if len(split) != 2 {
	// 	return startTime, endTime, err
	// }
	//
	// startTime, err = time.ParseInLocation(layout, split[0], location)
	// if err != nil {
	// 	return startTime, endTime, err
	// }
	//
	// date := startTime.Format("2/1-2006")
	// endTime, err = time.ParseInLocation(layout, date+" "+split[1], location)
	// if err != nil {
	// 	return startTime, endTime, err
	timePattern := `(\d{2}:\d{2}) - (\d{2}:\d{2})`

	// Find the time matches in the input string
	re := regexp.MustCompile(timePattern)
	matches := re.FindStringSubmatch(stamp)

	if len(matches) != 3 {
		return time.Time{}, 0, fmt.Errorf("Invalid input string format")
	}

	// Parse the start time and end time into time.Time objects
	sTime, err := time.Parse("15:04", matches[1])
	if err != nil {
		return time.Time{}, 0, err
	}

	eTime, err := time.Parse("15:04", matches[2])
	if err != nil {
		return time.Time{}, 0, err
	}

	duration := eTime.Sub(sTime)

	location, err := time.LoadLocation("Europe/Copenhagen")
	if err != nil {
		return time.Time{}, 0, err
	}

	return time.Date(date.Year(), date.Month(), date.Day(), sTime.Hour(), sTime.Minute(), 0, 0, location), duration, nil

	// return startTime, endTime, nil
}

func ParseTimeAndDate(moduleElement string, re *regexp.Regexp) (time.Time, time.Time, error) {
	var dateSlice []int

	// Format: DD/MM-YYYY HH:MM til HH:MM
	datesAndTimesSplit := re.Split(moduleElement, -1)

	for i, date := range datesAndTimesSplit {
		if i == 5 {
			continue // Ignore word "til"
		}
		int, err := strconv.Atoi(date)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		dateSlice = append(dateSlice, int)
	}

	// Format to time.Time
	startDate := time.Date(dateSlice[2], time.Month(dateSlice[1]), dateSlice[0], dateSlice[3], dateSlice[4], 0, 0, time.Local)
	endDate := time.Date(dateSlice[2], time.Month(dateSlice[1]), dateSlice[0], dateSlice[5], dateSlice[6], 0, 0, time.Local)

	return startDate, endDate, nil
}
