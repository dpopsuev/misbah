package daemon

import (
	"testing"
	"time"
)

func TestEventBus_SubscribeReceivesEvents(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe("test-container")
	defer bus.Unsubscribe("test-container", ch)

	bus.Publish(ContainerEvent{
		Type: EventStarted,
		Name: "test-container",
	})

	select {
	case ev := <-ch:
		if ev.Type != EventStarted {
			t.Fatalf("type = %q, want started", ev.Type)
		}
		if ev.Name != "test-container" {
			t.Fatalf("name = %q", ev.Name)
		}
		if ev.Timestamp == 0 {
			t.Fatal("timestamp should be auto-set")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventBus_WildcardReceivesAll(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe("") // wildcard
	defer bus.Unsubscribe("", ch)

	bus.Publish(ContainerEvent{Type: EventStarted, Name: "container-a"})
	bus.Publish(ContainerEvent{Type: EventExited, Name: "container-b"})

	ev1 := <-ch
	ev2 := <-ch
	if ev1.Name != "container-a" || ev2.Name != "container-b" {
		t.Fatalf("events = [%q, %q], want [container-a, container-b]", ev1.Name, ev2.Name)
	}
}

func TestEventBus_NameFilteredSubscriber(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe("only-this")
	defer bus.Unsubscribe("only-this", ch)

	bus.Publish(ContainerEvent{Type: EventStarted, Name: "other-container"})
	bus.Publish(ContainerEvent{Type: EventExited, Name: "only-this"})

	select {
	case ev := <-ch:
		if ev.Name != "only-this" {
			t.Fatalf("should only receive events for 'only-this', got %q", ev.Name)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestEventBus_UnsubscribeClosesChan(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe("x")
	bus.Unsubscribe("x", ch)

	// Channel should be closed.
	_, ok := <-ch
	if ok {
		t.Fatal("channel should be closed after unsubscribe")
	}
}

func TestEventBus_PublishNonBlocking(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe("x")
	defer bus.Unsubscribe("x", ch)

	// Fill the channel buffer (16).
	for i := 0; i < 20; i++ {
		bus.Publish(ContainerEvent{Type: EventStarted, Name: "x"})
	}

	// Should not block or panic — excess events dropped.
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	if count != 16 {
		t.Fatalf("received %d events, want 16 (buffer size)", count)
	}
}

func TestEventBus_ExitedEventHasExitCode(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe("x")
	defer bus.Unsubscribe("x", ch)

	bus.Publish(ContainerEvent{
		Type:     EventExited,
		Name:     "x",
		ExitCode: 42,
	})

	ev := <-ch
	if ev.ExitCode != 42 {
		t.Fatalf("exit code = %d, want 42", ev.ExitCode)
	}
}
