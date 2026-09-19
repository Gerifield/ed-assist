package input

import (
	"fmt"
	"strings"
)

// ScanCode represents a DirectInput keyboard scancode and whether it is an extended key.
type ScanCode struct {
	Code     uint16 `json:"code"`
	Extended bool   `json:"extended"`
}

// keyNameToScanCode maps Elite Dangerous key names (from .binds files) to DirectInput scan codes.
var keyNameToScanCode = map[string]ScanCode{
	// Alphanumeric keys
	"Key_Escape":       {Code: 0x01, Extended: false},
	"Key_1":            {Code: 0x02, Extended: false},
	"Key_2":            {Code: 0x03, Extended: false},
	"Key_3":            {Code: 0x04, Extended: false},
	"Key_4":            {Code: 0x05, Extended: false},
	"Key_5":            {Code: 0x06, Extended: false},
	"Key_6":            {Code: 0x07, Extended: false},
	"Key_7":            {Code: 0x08, Extended: false},
	"Key_8":            {Code: 0x09, Extended: false},
	"Key_9":            {Code: 0x0A, Extended: false},
	"Key_0":            {Code: 0x0B, Extended: false},
	"Key_Minus":        {Code: 0x0C, Extended: false},
	"Key_Equals":       {Code: 0x0D, Extended: false},
	"Key_Backspace":    {Code: 0x0E, Extended: false},
	"Key_Tab":          {Code: 0x0F, Extended: false},
	"Key_Q":            {Code: 0x10, Extended: false},
	"Key_W":            {Code: 0x11, Extended: false},
	"Key_E":            {Code: 0x12, Extended: false},
	"Key_R":            {Code: 0x13, Extended: false},
	"Key_T":            {Code: 0x14, Extended: false},
	"Key_Y":            {Code: 0x15, Extended: false},
	"Key_U":            {Code: 0x16, Extended: false},
	"Key_I":            {Code: 0x17, Extended: false},
	"Key_O":            {Code: 0x18, Extended: false},
	"Key_P":            {Code: 0x19, Extended: false},
	"Key_LeftBracket":  {Code: 0x1A, Extended: false},
	"Key_RightBracket": {Code: 0x1B, Extended: false},
	"Key_Enter":        {Code: 0x1C, Extended: false},
	"Key_LeftControl":  {Code: 0x1D, Extended: false},
	"Key_A":            {Code: 0x1E, Extended: false},
	"Key_S":            {Code: 0x1F, Extended: false},
	"Key_D":            {Code: 0x20, Extended: false},
	"Key_F":            {Code: 0x21, Extended: false},
	"Key_G":            {Code: 0x22, Extended: false},
	"Key_H":            {Code: 0x23, Extended: false},
	"Key_J":            {Code: 0x24, Extended: false},
	"Key_K":            {Code: 0x25, Extended: false},
	"Key_L":            {Code: 0x26, Extended: false},
	"Key_Semicolon":    {Code: 0x27, Extended: false},
	"Key_Apostrophe":   {Code: 0x28, Extended: false},
	"Key_Grave":        {Code: 0x29, Extended: false},
	"Key_LeftShift":    {Code: 0x2A, Extended: false},
	"Key_BackSlash":    {Code: 0x2B, Extended: false},
	"Key_Z":            {Code: 0x2C, Extended: false},
	"Key_X":            {Code: 0x2D, Extended: false},
	"Key_C":            {Code: 0x2E, Extended: false},
	"Key_V":            {Code: 0x2F, Extended: false},
	"Key_B":            {Code: 0x30, Extended: false},
	"Key_N":            {Code: 0x31, Extended: false},
	"Key_M":            {Code: 0x32, Extended: false},
	"Key_Comma":        {Code: 0x33, Extended: false},
	"Key_Period":       {Code: 0x34, Extended: false},
	"Key_Slash":        {Code: 0x35, Extended: false},
	"Key_RightShift":   {Code: 0x36, Extended: false},
	"Key_Numpad_Multiply": {Code: 0x37, Extended: false},
	"Key_LeftAlt":      {Code: 0x38, Extended: false},
	"Key_Space":        {Code: 0x39, Extended: false},
	"Key_CapsLock":     {Code: 0x3A, Extended: false},

	// Function keys
	"Key_F1":  {Code: 0x3B, Extended: false},
	"Key_F2":  {Code: 0x3C, Extended: false},
	"Key_F3":  {Code: 0x3D, Extended: false},
	"Key_F4":  {Code: 0x3E, Extended: false},
	"Key_F5":  {Code: 0x3F, Extended: false},
	"Key_F6":  {Code: 0x40, Extended: false},
	"Key_F7":  {Code: 0x41, Extended: false},
	"Key_F8":  {Code: 0x42, Extended: false},
	"Key_F9":  {Code: 0x43, Extended: false},
	"Key_F10": {Code: 0x44, Extended: false},
	"Key_F11": {Code: 0x57, Extended: false},
	"Key_F12": {Code: 0x58, Extended: false},

	// Keypad
	"Key_NumLock":         {Code: 0x45, Extended: false},
	"Key_ScrollLock":      {Code: 0x46, Extended: false},
	"Key_Numpad_7":        {Code: 0x47, Extended: false},
	"Key_Numpad_8":        {Code: 0x48, Extended: false},
	"Key_Numpad_9":        {Code: 0x49, Extended: false},
	"Key_Numpad_Subtract": {Code: 0x4A, Extended: false},
	"Key_Numpad_4":        {Code: 0x4B, Extended: false},
	"Key_Numpad_5":        {Code: 0x4C, Extended: false},
	"Key_Numpad_6":        {Code: 0x4D, Extended: false},
	"Key_Numpad_Add":      {Code: 0x4E, Extended: false},
	"Key_Numpad_1":        {Code: 0x4F, Extended: false},
	"Key_Numpad_2":        {Code: 0x50, Extended: false},
	"Key_Numpad_3":        {Code: 0x51, Extended: false},
	"Key_Numpad_0":        {Code: 0x52, Extended: false},
	"Key_Numpad_Decimal":  {Code: 0x53, Extended: false},
	"Key_Numpad_Divide":   {Code: 0x35, Extended: true},
	"Key_Numpad_Enter":    {Code: 0x1C, Extended: true},

	// Extended navigation keys
	"Key_RightControl": {Code: 0x1D, Extended: true},
	"Key_RightAlt":     {Code: 0x38, Extended: true},
	"Key_Home":         {Code: 0x47, Extended: true},
	"Key_UpArrow":      {Code: 0x48, Extended: true},
	"Key_PageUp":       {Code: 0x49, Extended: true},
	"Key_LeftArrow":    {Code: 0x4B, Extended: true},
	"Key_RightArrow":   {Code: 0x4D, Extended: true},
	"Key_End":          {Code: 0x4F, Extended: true},
	"Key_DownArrow":    {Code: 0x50, Extended: true},
	"Key_PageDown":     {Code: 0x51, Extended: true},
	"Key_Insert":       {Code: 0x52, Extended: true},
	"Key_Delete":       {Code: 0x53, Extended: true},
}

// LookupScanCode converts an Elite Dangerous key name (e.g. "Key_L", "Key_LeftControl")
// into its corresponding DirectInput hardware scancode.
func LookupScanCode(keyName string) (ScanCode, error) {
	if sc, ok := keyNameToScanCode[keyName]; ok {
		return sc, nil
	}

	// Case-insensitive lookup fallback
	for k, sc := range keyNameToScanCode {
		if strings.EqualFold(k, keyName) {
			return sc, nil
		}
	}

	return ScanCode{}, fmt.Errorf("unknown key name: %s", keyName)
}
