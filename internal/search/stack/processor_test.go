package stack

import (
	"testing"

	"github.com/lewtec/lewkit/x/driver"
	drvvulkan "github.com/lewtec/lewkit/x/driver/vulkan"
)

func TestChooseProcessorPrefersOnboard(t *testing.T) {
	handles := []driver.Handle[drvvulkan.Device]{
		{ID: "vulkan:nvidia:dedicated", Name: "NVIDIA GeForce RTX 3060", Weight: 70},
		{ID: "vulkan:amd:integrated", Name: "AMD Radeon Graphics", Weight: 50},
		{ID: "vulkan:mesa:software", Name: "llvmpipe", Weight: 0},
	}
	got, err := chooseProcessor(handles, "integrated")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "vulkan:amd:integrated" {
		t.Fatalf("id=%s want the onboard processor", got.ID)
	}
	got, err = chooseProcessor(handles, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "vulkan:amd:integrated" {
		t.Fatalf("default id=%s", got.ID)
	}
	got, err = chooseProcessor(handles, "dedicated")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "vulkan:nvidia:dedicated" {
		t.Fatalf("id=%s", got.ID)
	}
	got, err = chooseProcessor(handles, "vulkan:nvidia:dedicated")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "vulkan:nvidia:dedicated" {
		t.Fatalf("id=%s", got.ID)
	}
	if _, err := chooseProcessor(handles, "virtual"); err == nil {
		t.Fatal("missing type was accepted")
	}
}

func TestChooseProcessorWithoutOnboardUsesTheOtherDevice(t *testing.T) {
	handles := []driver.Handle[drvvulkan.Device]{
		{ID: "vulkan:nvidia:dedicated", Weight: 70},
	}
	got, err := chooseProcessor(handles, "integrated")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "vulkan:nvidia:dedicated" {
		t.Fatalf("id=%s", got.ID)
	}
}
