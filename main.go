package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/akamensky/argparse"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"github.com/xuri/excelize/v2"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		fmt.Println(v.Info.Sender.User)
		fmt.Printf("%s@%s:%s\n", v.Info.MessageSource.Chat.User, v.Info.MessageSource.Chat.Server, v.Message.GetConversation())
		fmt.Println("-----")
	}
}

func changeDirToOneContainingRunningBinary() {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("Can't get directory of running binary")
	}
	dirname := filepath.Dir(filename)
	os.Chdir(dirname)
}

type Location struct {
	name          string
	lasttimestamp int64
	jid           types.JID
	pickupPoints  []string
}

func readLocations() []Location {

	var locations []Location

	// Only run if it has been more than 8 hours of latest timestamp.
	content, err := os.ReadFile("last_timestamp.json")
	entries := make(map[string]int64)
	if err == nil {
		json.Unmarshal(content, &entries)
	}

	f, err := excelize.OpenFile("routes.xlsx")

	if err != nil {
		fmt.Println(err)
		return locations
	}
	defer func() {
		// Close the spreadsheet.
		if err := f.Close(); err != nil {
			fmt.Println(err)
		}
	}()

	cols, err := f.GetCols("Sheet1")
	if err != nil {
		fmt.Println(err)
		return locations
	}

	for _, col := range cols {
		var location Location
		location.pickupPoints = make([]string, 0)

		index := 0
		for _, rowCell := range col {
			if len(rowCell) == 0 {
				continue
			}
			if index == 0 {
				location.name = rowCell
			} else if index == 1 {
				location.jid, err = types.ParseJID(rowCell)
				if err != nil {
					fmt.Println("Failed to parse JID: ", rowCell)
				}
			} else {
				location.pickupPoints = append(location.pickupPoints, rowCell)
			}

			index += 1
		}
		if len(location.name) != 0 {
			locations = append(locations, location)
		}
	}

	for i := 0; i < len(locations); i++ {
		locations[i].lasttimestamp = entries[locations[i].jid.String()]
	}

	return locations
}

func writeLasttimestamps(locations []Location) {
	entries := make(map[string]int64)
	for _, location := range locations {
		entries[location.jid.String()] = location.lasttimestamp
	}
	content, err := json.Marshal(entries)
	if err == nil {
		os.WriteFile("last_timestamp.json", content, 0644)
	}
}

func createClient() (*whatsmeow.Client, *sqlstore.Container) {

	dbLog := waLog.Stdout("Database", "WARN", true)
	// Make sure you add appropriate DB connector imports, e.g. github.com/mattn/go-sqlite3 for SQLite
	container, err := sqlstore.New("sqlite3", "file:examplestore.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}

	// If you want multiple sessions, remember their JIDs and use .GetDevice(jid) or .GetAllDevices() instead.
	deviceStore, err := container.GetFirstDevice()
	if err != nil {
		panic(err)
	}
	clientLog := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	client.AddEventHandler(eventHandler)

	if client.Store.ID == nil {
		fmt.Println("Client store ID is nil, scanning QR")
		// 	// No ID stored, new login
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				// Render the QR code here
				// e.g. qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				// or just manually `echo 2@... | qrencode -t ansiutf8` in a terminal
				fmt.Println("QR code:", evt.Code)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		// Already logged in, just connect
		fmt.Println("Connecting")

		err = client.Connect()
		if err != nil {
			panic(err)
		}
	}

	return client, container
}
func main() {
	parser := argparse.NewParser("whats_app_poll", "A great app to send polls to different groups at the same time")
	keepalive := parser.Flag("K", "keepalive", &argparse.Options{Default: false})
	parser.Parse(os.Args)
	changeDirToOneContainingRunningBinary()

	locations := readLocations()
	allSent := true
	for _, location := range locations {
		if canSend(location.lasttimestamp) {
			allSent = false
		} else {
			println("Already sent poll for: ", location.name, time.Unix(location.lasttimestamp, 0).String())

		}
	}

	var client *whatsmeow.Client = nil
	var container *sqlstore.Container = nil
	if !allSent || *keepalive {
		client, container = createClient()
	}

	if !allSent {
		for i := 0; i < len(locations); i++ {
			fmt.Println("----Start Location----")
			fmt.Println("Sending to: ", locations[i])
			sendLocation(client, &locations[i])
			time.Sleep(5 * time.Second)
			fmt.Println("----End Location----")
		}

		writeLasttimestamps(locations)
	}
	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
	if *keepalive {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
	}
	if client != nil {
		client.Disconnect()
	}
	if container != nil {
		container.Close()
	}
}

func canSend(lasttimestamp int64) bool {
	timeDifference := 12 * time.Hour
	already_sent := false

	if lasttimestamp+int64(timeDifference.Seconds()) > time.Now().Unix() {
		already_sent = true
	}
	return !already_sent
}

func sendLocation(client *whatsmeow.Client, loc *Location) {
	already_sent := false

	if !canSend(loc.lasttimestamp) {
		already_sent = true
	}

	if !already_sent {
		// A hectic trial to wait for a few seconds, to see if it will update the Keys.
		time.Sleep(10 * time.Second)

		currentTime := time.Now()

		// If after 8 AM, simply we are generating for the next day
		if currentTime.Hour() >= 8 {

			currentTime = currentTime.AddDate(0, 0, 1)
		}

		currentTime = time.Date(
			currentTime.Year(), currentTime.Month(), currentTime.Day(), 0, 0, 0, 0, currentTime.Location())

		optionNames := loc.pickupPoints
		fmt.Println("Options are: ", optionNames)
		headline := "Auto-generated: " + fmt.Sprintf("%d/%d/%d", currentTime.Day(), currentTime.Month(), currentTime.Year())

		lastSuccess := sendPoll(client, loc.jid, headline, optionNames)
		if currentTime.Weekday() == time.Friday {
			// Send for Saturday
			currentTime = currentTime.AddDate(0, 0, 1)
			headline = "Auto-generated: " + fmt.Sprintf("%d/%d/%d", currentTime.Day(), currentTime.Month(), currentTime.Year())
			lastSuccess = sendPoll(client, loc.jid, headline, optionNames)

			// Send for Sunday
			currentTime = currentTime.AddDate(0, 0, 1)
			headline = "Auto-generated: " + fmt.Sprintf("%d/%d/%d", currentTime.Day(), currentTime.Month(), currentTime.Year())
			lastSuccess = sendPoll(client, loc.jid, headline, optionNames)

		}

		time.Sleep(5 * time.Second)

		if lastSuccess {

			loc.lasttimestamp = currentTime.Unix()

		}
	}
}
func sendPoll(client *whatsmeow.Client, gJID types.JID, headline string, optionNames []string) bool {

	fmt.Println(headline)
	pollMessage := client.BuildPollCreation(headline, optionNames, 1)

	fmt.Println("Create Poll Message succuessfully  : ", pollMessage)

	_, err := client.SendMessage(context.Background(), gJID, pollMessage)
	if err == nil {
		fmt.Println("Sent Poll Succuessfully ", gJID)
		return true
	} else {
		fmt.Println("Failed to Send Poll", gJID)
		fmt.Println(err)
		return false
	}
}
