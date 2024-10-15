package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		fmt.Println("Received a message from !", v.Info.Sender)

		fmt.Println("Received a message!", v.Message.GetConversation())
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

func main() {
	changeDirToOneContainingRunningBinary()

	// Only run if it has been more than 8 hours of latest timestamp.
	content, err := os.ReadFile("last_timestamp.txt")
	var lastTimestamp int64 = 0
	if err == nil {
		contentstr := string(content)
		contentstr = strings.TrimSpace(contentstr)
		lastTimestamp, _ = strconv.ParseInt(contentstr, 10, 64)

	}
	timeDifference := 12 * time.Hour
	if lastTimestamp+int64(timeDifference.Seconds()) > time.Now().Unix() {
		println("Already sent poll for: ", time.Unix(lastTimestamp, 0).String())
		return
	}

	dbLog := waLog.Stdout("Database", "DEBUG", true)
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
	clientLog := waLog.Stdout("Client", "DEBUG", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	// client.AddEventHandler(eventHandler)

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

	// A hectic trial to wait for a few seconds, to see if it will update the Keys.
	time.Sleep(10 * time.Second)

	strJID := "120363048809400922@g.us"
	var gJID types.JID
	gJID, err = types.ParseJID(strJID)
	if err != nil {
		fmt.Println("Failed to parse JID: ", strJID)
	} else {
		fmt.Println("Parsed JID correctly", gJID)

	}

	currentTime := time.Now()

	var daysShift int = -1

	if len(os.Args) > 1 {
		daysShift, err = strconv.Atoi(os.Args[1])
		if err != nil {
			fmt.Println("Failed to parse days shift param: ", os.Args[1])
			os.Exit(-1)
		}

		fmt.Println("Parsed days shift param: ", daysShift)
		currentTime = currentTime.AddDate(0, 0, daysShift)

	} else {
		// If after 8 AM, simply we are generating for the next day
		if currentTime.Hour() >= 8 {

			currentTime = currentTime.AddDate(0, 0, 1)
		}
	}
	currentTime = time.Date(
		currentTime.Year(), currentTime.Month(), currentTime.Day(), 0, 0, 0, 0, currentTime.Location())

	file, err := os.Open("options.txt")
	if err != nil {
		fmt.Println("Failed to open option.txt file: ", err)
		os.Exit(-1)
	}
	defer file.Close()

	var optionNames []string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		optionNames = append(optionNames, scanner.Text())
	}
	fmt.Println("Options are: ", optionNames)
	headline := "Auto-generated: " + fmt.Sprintf("%d/%d/%d", currentTime.Day(), currentTime.Month(), currentTime.Year())

	lastSuccess := sendPoll(client, gJID, headline, optionNames)
	if currentTime.Weekday() == time.Friday {
		// Send for Saturday
		currentTime = currentTime.AddDate(0, 0, 1)
		headline = "Auto-generated: " + fmt.Sprintf("%d/%d/%d", currentTime.Day(), currentTime.Month(), currentTime.Year())
		lastSuccess = sendPoll(client, gJID, headline, optionNames)

		// Send for Sunday
		currentTime = currentTime.AddDate(0, 0, 1)
		headline = "Auto-generated: " + fmt.Sprintf("%d/%d/%d", currentTime.Day(), currentTime.Month(), currentTime.Year())
		lastSuccess = sendPoll(client, gJID, headline, optionNames)

	}

	time.Sleep(5 * time.Second)
	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
	// c := make(chan os.Signal, 1)
	// signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	// <-c

	client.Disconnect()
	container.Close()

	if lastSuccess {

		lastTimestampFile, err := os.Create("last_timestamp.txt")
		if err == nil {
			lastTimestampFile.WriteString(strconv.FormatInt(currentTime.Unix(), 10))
		}

		lastTimestampFile.Close()
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
		return false
	}
}
