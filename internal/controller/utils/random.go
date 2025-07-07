/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"math/rand"
	"time"
)

// RandomGenerator provides utilities for generating random strings and values
type RandomGenerator struct{}

// NewRandomGenerator creates a new RandomGenerator instance
func NewRandomGenerator() *RandomGenerator {
	return &RandomGenerator{}
}

// RandString generates a random string of specified length using lowercase letters and numbers
func (r *RandomGenerator) RandString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	rand.Seed(time.Now().UnixNano())
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// RandStringWithPrefix generates a random string with a prefix
func (r *RandomGenerator) RandStringWithPrefix(prefix string, length int) string {
	return prefix + r.RandString(length)
}

// RandStringAlphanumeric generates a random alphanumeric string
func (r *RandomGenerator) RandStringAlphanumeric(n int) string {
	return r.RandString(n)
}

// RandStringNumeric generates a random numeric string
func (r *RandomGenerator) RandStringNumeric(n int) string {
	numbers := []rune("0123456789")
	rand.Seed(time.Now().UnixNano())
	b := make([]rune, n)
	for i := range b {
		b[i] = numbers[rand.Intn(len(numbers))]
	}
	return string(b)
}

// RandInt generates a random integer between min and max (inclusive)
func (r *RandomGenerator) RandInt(min, max int) int {
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(max-min+1) + min
}
