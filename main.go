package main

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/AverageStardust/simple-io/input"
	"github.com/AverageStardust/simple-io/output"
	"github.com/bendahl/uinput"
	"github.com/splace/joysticks"
)

type BalanceBoard struct {
	sensors    [4]float64
	sum        float64
	rollingSum float64
	x          float64
	y          float64
	pressUntil time.Duration
	gamepad    uinput.Gamepad
}

type OutputSource interface {
	SetGamepad(gamepad uinput.Gamepad)
}

const kgsPerBalanceUnit = 361

var balanceSensorOffsets = [4]float64{1, 1, 1, 0.9982}
var errDeviceNotFound = errors.New("Device not found")

func main() {
	for {
		err := loop()
		if err != nil {
			input.Acknowledge(fmt.Sprintf("err: %v", err))
		} else {
			break
		}
	}
}

func loop() error {
	choice := input.NewChoice("Balance Board", "Exit")
	for {
		output.EraseScreen()
		output.MoveToHome()
		switch choice.Ask().ResultIndex() {
		case 0:
			err := runWithGamepad("Virtual 360 Controller (Balance Board)", remapBalanceBoard)
			if err != nil {
				return err
			}
		case 1:
			return nil
		}
	}
}

func runWithGamepad(name string, remapper func() (OutputSource, error)) error {
	board, err := remapper()
	if err != nil {
		return err
	}

	gamepad, err := uinput.CreateGamepad("/dev/uinput", []byte(name), 0x0738, 0x4726)
	if err != nil {
		return err
	}
	defer gamepad.Close()

	board.SetGamepad(gamepad)

	input.Acknowledge("Remapping active...")

	return nil
}

func remapBalanceBoard() (OutputSource, error) {
	device, err := findDevice(1, 4)
	if err != nil {
		return nil, err
	}

	board := &BalanceBoard{}

	go device.ParcelOutEvents()

	rightChan := device.OnMove(1)
	leftChan := device.OnMove(2)
	// buttonChan := device.OnButton(1) // power button

	go func() {
		for {
			select {
			case event := <-rightChan:
				board.ProcessInput(event.(joysticks.CoordsEvent), false)

			case event := <-leftChan:
				board.ProcessInput(event.(joysticks.CoordsEvent), true)
			}
		}
	}()

	return board, nil
}

func findDevice(buttons, axes int) (device *joysticks.HID, err error) {
	for i := range 255 {
		if joysticks.DeviceExists(uint8(i + 1)) {

			fmt.Printf("Trying to connect to device %d...\n", i)
			device = joysticks.Connect(i + 1)

			if buttons == len(device.Buttons) && axes == len(device.HatAxes) {
				fmt.Printf("Device %d matches!\n", i)
				return device, nil
			} else {
				fmt.Printf("Device %d does not match (%d buttons and %d hats)\n", i, len(device.Buttons), len(device.HatAxes))
			}
		}
	}

	return nil, errDeviceNotFound
}

func (board *BalanceBoard) SetGamepad(gamepad uinput.Gamepad) {
	board.gamepad = gamepad
}

func (board *BalanceBoard) ProcessInput(event joysticks.CoordsEvent, isLeft bool) {
	if isLeft {
		board.sensors[0] = max(balanceSensorOffsets[0]+float64(event.X), 0) * kgsPerBalanceUnit
		board.sensors[2] = max(balanceSensorOffsets[2]+float64(event.Y), 0) * kgsPerBalanceUnit
	} else {
		board.sensors[1] = max(balanceSensorOffsets[1]+float64(event.X), 0) * kgsPerBalanceUnit
		board.sensors[3] = max(balanceSensorOffsets[3]+float64(event.Y), 0) * kgsPerBalanceUnit
	}

	board.sum = board.sensors[0] + board.sensors[1] + board.sensors[2] + board.sensors[3]
	board.rollingSum = board.rollingSum*0.98 + board.sum*0.02

	if board.sum < 5 {
		board.x = 0
		board.y = 0
	} else {
		board.x = (board.sensors[1] + board.sensors[3] - board.sensors[0] - board.sensors[2]) / board.sum
		board.y = (board.sensors[2] + board.sensors[3] - board.sensors[0] - board.sensors[1]) / board.sum

		// normalize
		length := math.Hypot(board.x, board.y)

		if length < 0.2 {
			board.x = 0
			board.y = 0
		} else {
			board.x /= length
			board.y /= length
		}
	}

	if board.rollingSum > 5 && board.sum < board.rollingSum*0.75 {
		board.rollingSum = max(0, board.rollingSum-25)
		if board.pressUntil < event.Time {
			board.pressUntil = event.Time + time.Millisecond*350
		} else {
			board.pressUntil += time.Millisecond * 250
		}
	}

	if board.gamepad != nil {
		if board.pressUntil > event.Time {
			board.gamepad.ButtonDown(uinput.ButtonSouth)
		} else {
			board.gamepad.ButtonUp(uinput.ButtonSouth)
		}

		board.gamepad.LeftStickMove(float32(board.x), float32(board.y))
	}
}
