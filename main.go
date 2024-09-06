package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
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

func main() {
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
	client.AddEventHandler(eventHandler)

	if client.Store.ID == nil {
		fmt.Println("Client store ID is nil, scanning QR")
		// No ID stored, new login
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

	strJID := "120363048809400922@g.us"
	var gJID types.JID
	gJID, err = types.ParseJID(strJID)
	if err != nil {
		fmt.Println("Failed to parse JID: ", strJID)
	} else {
		fmt.Println("Parsed JID correctly", gJID)

	}

	var optionNames []string
	optionNames = append(optionNames, "القاهرة للمبيعات")
	optionNames = append(optionNames, "الف مسكن")
	optionNames = append(optionNames, "حجاز - الملف")
	optionNames = append(optionNames, "حجاز - البنك")
	optionNames = append(optionNames, "هليوبوليس")
	optionNames = append(optionNames, "روكسي")
	optionNames = append(optionNames, "كوبري التجنيد")
	optionNames = append(optionNames, "كوبري المطرية")

	currentTime := time.Now()
	var headline string

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

	headline = "Auto-generated: " + fmt.Sprintf("%d/%d/%d", currentTime.Day(), currentTime.Month(), currentTime.Year())

	fmt.Println(headline)
	pollMessage := client.BuildPollCreation(headline, optionNames, 1)

	fmt.Println("Create Poll Message succuessfully  : ", pollMessage)

	_, err = client.SendMessage(context.Background(), gJID, pollMessage)
	if err != nil {
		fmt.Println("Sent Poll Succuessfully ", strJID)
	} else {
		fmt.Println("Failed to Send Poll", gJID)

	}

	time.Sleep(5 * time.Second)
	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
	// c := make(chan os.Signal, 1)
	// signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	// <-c

	client.Disconnect()
}
