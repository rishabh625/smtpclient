package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	smtpi "smtpclient/smtp"
	"sync"
)

var fname, host, rcpt, from, username, password string
var session, count int

func main() {
	//smtpi.TestSmtpi()
	flag.StringVar(&host, "--host", "stage2.pepipost.com:25", "Host name : port of smtp")
	flag.StringVar(&fname, "--filepath", "email.eml", "File name")
	flag.StringVar(&rcpt, "--rcpt", "rishabhmishra131@gmail.com", "From Email")
	flag.StringVar(&from, "--from", "newpricing04@pepitest.xyz", "Recipient")
	flag.StringVar(&username, "--username", "", "Username")
	flag.StringVar(&password, "--password", "", "Password")
	flag.IntVar(&session, "--session", 1, "Parallel smtp session")
	flag.IntVar(&count, "--count", 1, "No of messages per session")
	flag.Parse()
	// cfg := &tls.Config{
	// 	MinVersion:               tls.VersionTLS12,
	// 	CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
	// 	PreferServerCipherSuites: true,
	// 	CipherSuites: []uint16{
	// 		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	// 		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	// 		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	// 		tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	// 	},
	// 	InsecureSkipVerify: false,
	// 	ServerName:         host,
	// }
	//c.StartTLS(cfg)
	//count := 1
	wg := sync.WaitGroup{}
	for i := 0; i < session; i++ {
		fmt.Printf("Sending message %d\n", i)
		c := smtpConnection(host, username, password)
		con, err := ioutil.ReadFile(fname)
		if err != nil {
			fmt.Println("Failed to read file")
		}
		wg.Add(1)
		go func() {
			for k := 0; k < count; k++ {
				sendMail(c, from, rcpt, "", con)
			}
			//log.Printf("Trying to reconnect to smtp server")
			wg.Done()
		}()
	}
	wg.Wait()
}

func quarantineMail() {

}
func smtpConnection(server string, username, password string) *smtpi.Client {
	c, err := smtpi.Dial(server, "localhost")
	if err != nil {
		log.Fatal(err)
	}
	if username != "" && password != "" {
		auth := smtpi.PlainAuth("", "user@example.com", "password", server)

		err := c.Auth(auth)
		if err != nil {
			log.Println("Failed to authenticate")
			log.Panic(err)
		}
	}
	return c
}

func sendMail(c *smtpi.Client, from string, to string, fname string, con []byte) int {
	// Set the sender and recipient first
	if err := c.Mail(from); err != nil {
		log.Printf("Could not start mail from %s %v\n", from, err)
		return 1
	}
	if err := c.Rcpt(to); err != nil {
		log.Printf("Could not send rcpt to %s %v\n", to, err)
		return 2
	}

	// Send the email body.
	wc, err := c.Data()
	if err != nil {
		log.Printf("Could not start data %v\n", err)
		return 1
	}
	_, err = wc.Write(con)
	//_, err = fmt.Fprintf(wc, string(con))
	if err != nil {
		log.Printf("Could not send data %v\n", err)
		return 1
	}
	err = wc.Close()
	if err == nil {
		log.Printf("Sent message DSN=<%s>", c.ResponseMessage)
	} else {
		log.Printf("No Response", err.Error())
	}

	return 0
}
