// +build windows

//Winlog hooks into the Windows Event Log and streams events through channels
package winlog

import (
	"fmt"
	"time"
)

/* WinLogWatcher encompasses the overall functionality, eventlog subscriptions etc. */

// Event Channel for receiving events
func (wlw *WinLogWatcher) Event() <-chan *WinLogEvent {
	return wlw.eventChan
}

// Channel for receiving errors (not "error" events)
func (wlw *WinLogWatcher) Error() <-chan error {
	return wlw.errChan
}

// NewWinLogWatcher creates a new watcher
func NewWinLogWatcher() (*WinLogWatcher, error) {
	cHandle, err := GetSystemRenderContext()
	if err != nil {
		return nil, err
	}
	return &WinLogWatcher{
		shutdown:       make(chan interface{}),
		errChan:        make(chan error),
		eventChan:      make(chan *WinLogEvent),
		renderContext:  cHandle,
		watches:        make(map[string]*channelWatcher),
		RenderKeywords: true,
		RenderMessage:  true,
		RenderLevel:    true,
		RenderTask:     true,
		RenderProvider: true,
		RenderOpcode:   true,
		RenderChannel:  true,
		RenderId:       true,
	}, nil
}

// Subscribe to a Windows Event Log channel, starting with the first event
// in the log. `query` is an XPath expression for filtering events: to recieve
// all events on the channel, use "*" as the query.
func (self *WinLogWatcher) SubscribeFromBeginning(channel, query string) error {
	return self.subscribeWithoutBookmark(channel, query, EvtSubscribeStartAtOldestRecord)
}

// Subscribe to a Windows Event Log channel, starting with the next event
// that arrives. `query` is an XPath expression for filtering events: to recieve
// all events on the channel, use "*" as the query.
func (self *WinLogWatcher) SubscribeFromNow(channel, query string) error {
	return self.subscribeWithoutBookmark(channel, query, EvtSubscribeToFutureEvents)
}

func (self *WinLogWatcher) subscribeWithoutBookmark(channel, query string, flags EVT_SUBSCRIBE_FLAGS) error {
	self.watchMutex.Lock()
	defer self.watchMutex.Unlock()
	if _, ok := self.watches[channel]; ok {
		return fmt.Errorf("A watcher for channel %q already exists", channel)
	}
	newBookmark, err := CreateBookmark()
	if err != nil {
		return fmt.Errorf("Failed to create new bookmark handle: %v", err)
	}
	callback := &LogEventCallbackWrapper{callback: self, subscribedChannel: channel}
	subscription, err := CreateListener(channel, query, flags, callback)
	if err != nil {
		CloseEventHandle(uint64(newBookmark))
		return err
	}
	self.watches[channel] = &channelWatcher{
		bookmark:     newBookmark,
		subscription: subscription,
		callback:     callback,
	}
	return nil
}

// Subscribe to a Windows Event Log channel, starting with the first event in the log
// after the bookmarked event. There may be a gap if events have been purged. `query`
// is an XPath expression for filtering events: to recieve all events on the channel,
// use "*" as the query
func (self *WinLogWatcher) SubscribeFromBookmark(channel, query string, xmlString string) error {
	self.watchMutex.Lock()
	defer self.watchMutex.Unlock()
	if _, ok := self.watches[channel]; ok {
		return fmt.Errorf("A watcher for channel %q already exists", channel)
	}
	callback := &LogEventCallbackWrapper{callback: self, subscribedChannel: channel}
	bookmark, err := CreateBookmarkFromXml(xmlString)
	if err != nil {
		return fmt.Errorf("Failed to create new bookmark handle: %v", err)
	}
	subscription, err := CreateListenerFromBookmark(channel, query, callback, bookmark)
	if err != nil {
		CloseEventHandle(uint64(bookmark))
		return fmt.Errorf("Failed to add listener: %v", err)
	}
	self.watches[channel] = &channelWatcher{
		bookmark:     bookmark,
		subscription: subscription,
		callback:     callback,
	}
	return nil
}

/* Remove subscription from channel */
func (self *WinLogWatcher) RemoveSubscription(channel string) error {
	self.watchMutex.Lock()
	defer self.watchMutex.Unlock()

	var cancelErr, closeErr error
	if watch, ok := self.watches[channel]; ok {
		cancelErr = CancelEventHandle(uint64(watch.subscription))
		closeErr = CloseEventHandle(uint64(watch.subscription))
		CloseEventHandle(uint64(watch.bookmark))
	}

	delete(self.watches, channel)
	if cancelErr != nil {
		return cancelErr
	}
	return closeErr
}

// Remove all subscriptions from this watcher and shut down.
func (self *WinLogWatcher) Shutdown() {
	close(self.shutdown)
	for channel := range self.watches {
		self.RemoveSubscription(channel)
	}
	CloseEventHandle(uint64(self.renderContext))
	close(self.errChan)
	close(self.eventChan)
}

/* Publish the received error to the errChan, but discard if shutdown is in progress */
func (self *WinLogWatcher) PublishError(err error) {
	select {
	case self.errChan <- err:
	case <-self.shutdown:
	}
}

func (self *WinLogWatcher) convertEvent(handle EventHandle, subscribedChannel string) (*WinLogEvent, error) {
	// Rendered values
	var computerName, providerName, channel string
	var level, task, opcode, recordId, qualifiers, eventId, processId, threadId, version uint64
	var created time.Time

	// Localized fields
	var keywordsText, msgText, lvlText, taskText, providerText, opcodeText, channelText, idText string

	// Publisher fields
	var publisherHandle PublisherHandle
	var publisherHandleErr error

	// Render the values
	renderedFields, renderedFieldsErr := RenderEventValues(self.renderContext, handle)
	xml, xmlErr := RenderEventXML(handle)

	if renderedFieldsErr == nil {
		// If fields don't exist we include the nil value
		computerName, _ = renderedFields.String(EvtSystemComputer)
		providerName, _ = renderedFields.String(EvtSystemProviderName)
		channel, _ = renderedFields.String(EvtSystemChannel)
		level, _ = renderedFields.Uint(EvtSystemLevel)
		task, _ = renderedFields.Uint(EvtSystemTask)
		opcode, _ = renderedFields.Uint(EvtSystemOpcode)
		recordId, _ = renderedFields.Uint(EvtSystemEventRecordId)
		qualifiers, _ = renderedFields.Uint(EvtSystemQualifiers)
		eventId, _ = renderedFields.Uint(EvtSystemEventID)
		processId, _ = renderedFields.Uint(EvtSystemProcessID)
		threadId, _ = renderedFields.Uint(EvtSystemThreadID)
		version, _ = renderedFields.Uint(EvtSystemVersion)
		created, _ = renderedFields.FileTime(EvtSystemTimeCreated)

		// Render localized fields
		publisherHandle, publisherHandleErr = GetEventPublisherHandle(renderedFields)
		if publisherHandleErr == nil {

			if self.RenderKeywords {
				keywordsText, _ = FormatMessage(publisherHandle, handle, EvtFormatMessageKeyword)
			}

			if self.RenderMessage {
				msgText, _ = FormatMessage(publisherHandle, handle, EvtFormatMessageEvent)
			}

			if self.RenderLevel {
				lvlText, _ = FormatMessage(publisherHandle, handle, EvtFormatMessageLevel)
			}

			if self.RenderTask {
				taskText, _ = FormatMessage(publisherHandle, handle, EvtFormatMessageTask)
			}

			if self.RenderProvider {
				providerText, _ = FormatMessage(publisherHandle, handle, EvtFormatMessageProvider)
			}

			if self.RenderOpcode {
				opcodeText, _ = FormatMessage(publisherHandle, handle, EvtFormatMessageOpcode)
			}

			if self.RenderChannel {
				channelText, _ = FormatMessage(publisherHandle, handle, EvtFormatMessageChannel)
			}

			if self.RenderId {
				idText, _ = FormatMessage(publisherHandle, handle, EvtFormatMessageId)
			}
		}
	}

	CloseEventHandle(uint64(publisherHandle))

	event := WinLogEvent{
		Xml:               xml,
		XmlErr:            xmlErr,
		ProviderName:      providerName,
		EventId:           eventId,
		Qualifiers:        qualifiers,
		Level:             level,
		Task:              task,
		Opcode:            opcode,
		Created:           created,
		RecordId:          recordId,
		ProcessId:         processId,
		ThreadId:          threadId,
		Channel:           channel,
		ComputerName:      computerName,
		Version:           version,
		RenderedFieldsErr: renderedFieldsErr,

		Keywords:           keywordsText,
		Msg:                msgText,
		LevelText:          lvlText,
		TaskText:           taskText,
		OpcodeText:         opcodeText,
		ChannelText:        channelText,
		ProviderText:       providerText,
		IdText:             idText,
		PublisherHandleErr: publisherHandleErr,

		SubscribedChannel: subscribedChannel,
	}
	return &event, nil
}

/* Publish a new event */
func (self *WinLogWatcher) PublishEvent(handle EventHandle, subscribedChannel string) {

	// Convert the event from the event log schema
	event, err := self.convertEvent(handle, subscribedChannel)
	if err != nil {
		self.PublishError(err)
		return
	}

	// Get the bookmark for the channel
	self.watchMutex.Lock()
	watch, ok := self.watches[subscribedChannel]
	self.watchMutex.Unlock()
	if !ok {
		self.errChan <- fmt.Errorf("No handle for channel bookmark %q", subscribedChannel)
		return
	}

	// Update the bookmark with the current event
	UpdateBookmark(watch.bookmark, handle)

	// Serialize the boomark as XML and include it in the event
	bookmarkXml, err := RenderBookmark(watch.bookmark)
	if err != nil {
		self.PublishError(fmt.Errorf("Error rendering bookmark for event - %v", err))
		return
	}
	event.Bookmark = bookmarkXml

	// Don't block when shutting down if the consumer has gone away
	select {
	case self.eventChan <- event:
	case <-self.shutdown:
		return
	}

}
