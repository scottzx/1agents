package tunnel

import (
	"fmt"
	"log"

	"github.com/skip2/go-qrcode"
)

// RenderTerminalQR generates a high-contrast ANSI/ASCII QR code in the terminal.
// It uses small character blocks to fit nicely in standard console sizes.
func RenderTerminalQR(content string) {
	qr, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		log.Printf("[tunnel] Failed to generate QR code: %v", err)
		return
	}

	// ToSmallString(false) uses half-block characters (▄, ▀, █) to render
	// the QR code in half the vertical height, which is perfect for terminals.
	qrString := qr.ToSmallString(false)
	
	fmt.Println()
	fmt.Println(qrString)
	fmt.Println()
}
