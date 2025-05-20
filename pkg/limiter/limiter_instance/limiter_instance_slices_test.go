package limiter_instance

import (
	"github.com/google/go-cmp/cmp"
	"github.com/samber/lo"
	"testing"
)

func TestDiscardFirstItems(t *testing.T) {
	slice := []int{1, 2, 3, 4, 5}
	expected := append([]int{}, slice[3:]...)
	actual := discardFirstItems(slice, 3)
	if cmp.Diff(expected, actual) != "" {
		t.Errorf("Expected %v, got %v", expected, actual)
	}
}

func TestDiscardFirstItems_many(t *testing.T) {
	slice := lo.Range(1000)
	expected := append([]int{}, slice[565:]...)
	actual := discardFirstItems(slice, 565)
	if cmp.Diff(expected, actual) != "" {
		t.Errorf("Expected %v, got %v", expected, actual)
	}
}

func TestDiscardFirstItems_all(t *testing.T) {
	slice := lo.Range(1000)
	expected := append([]int{}, slice[1000:]...)
	actual := discardFirstItems(slice, 1000)
	if cmp.Diff(expected, actual) != "" {
		t.Errorf("Expected %v, got %v", expected, actual)
	}
}

func TestDiscardFirstItems_one(t *testing.T) {
	slice := lo.Range(10)
	expected := append([]int{}, slice[1:]...)
	actual := discardFirstItems(slice, 1)
	if cmp.Diff(expected, actual) != "" {
		t.Errorf("Expected %v, got %v", expected, actual)
	}
}

func TestDiscardFirstItems_none(t *testing.T) {
	slice := lo.Range(1000)
	expected := slice[0:]
	actual := discardFirstItems(slice, 0)
	if cmp.Diff(expected, actual) != "" {
		t.Errorf("Expected %v, got %v", expected, actual)
	}
}

func TestDiscardAt(t *testing.T) {
	slice := []int{1, 2, 3, 4, 5}
	expected := []int{1, 2, 4, 5}
	actual := discardItemAt(slice, 2)
	if cmp.Diff(expected, actual) != "" {
		t.Errorf("Expected %v, got %v", expected, actual)
	}
}

func TestDiscardAt_head(t *testing.T) {
	slice := []int{1, 2, 3, 4, 5}
	expected := []int{2, 3, 4, 5}
	actual := discardItemAt(slice, 0)
	if cmp.Diff(expected, actual) != "" {
		t.Errorf("Expected %v, got %v", expected, actual)
	}
}

func TestDiscardAt_tail(t *testing.T) {
	slice := []int{1, 2, 3, 4, 5}
	expected := []int{1, 2, 3, 4}
	actual := discardItemAt(slice, 4)
	if cmp.Diff(expected, actual) != "" {
		t.Errorf("Expected %v, got %v", expected, actual)
	}
}
