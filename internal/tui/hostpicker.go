package tui

import (
	"fmt"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
)

const iconServer = "󰒋 "

type hostItem struct {
	host sshpkg.Host
}

func (h hostItem) Title() string       { return iconServer + h.host.Name }
func (h hostItem) Description() string { return h.host.User + "@" + h.host.Hostname + ":" + h.host.Port }
func (h hostItem) FilterValue() string { return h.host.Name + " " + h.host.Hostname }

type HostPicker struct {
	list     list.Model
	chosen   *sshpkg.Host
	quitting bool
}

var titleStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("62")).
	Foreground(lipgloss.Color("230")).
	Padding(0, 1)

func NewHostPicker(hosts []sshpkg.Host) HostPicker {
	items := make([]list.Item, len(hosts))
	for i, h := range hosts {
		items[i] = hostItem{h}
	}

	l := list.New(items, list.NewDefaultDelegate(), 60, 20)
	l.Title = "Select SSH Host"
	l.Styles.Title = titleStyle
	l.SetFilteringEnabled(true)
	l.SetFilterState(list.Filtering)

	return HostPicker{list: l}
}

func (m HostPicker) Init() tea.Cmd {
	return nil
}

func (m HostPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "q":
			// Don't quit while the user is typing in the filter input.
			if m.list.FilterState() != list.Filtering {
				m.quitting = true
				return m, tea.Quit
			}
		case "enter":
			if item, ok := m.list.SelectedItem().(hostItem); ok {
				h := item.host
				m.chosen = &h
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m HostPicker) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	return tea.NewView("\n" + m.list.View())
}

func RunHostPicker(hosts []sshpkg.Host) (*sshpkg.Host, error) {
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no SSH hosts found in ~/.ssh/config")
	}

	p := tea.NewProgram(NewHostPicker(hosts))
	m, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("host picker: %w", err)
	}

	result := m.(HostPicker)
	if result.chosen == nil {
		return nil, fmt.Errorf("no host selected")
	}
	return result.chosen, nil
}

// dirPickerMsg carries SSH client connection after host is selected
type ConnectedMsg struct {
	Client *sshpkg.Client
	Host   sshpkg.Host
}

func connectCmd(host sshpkg.Host) tea.Cmd {
	return func() tea.Msg {
		client, err := sshpkg.Connect(host)
		if err != nil {
			return errMsg{err}
		}
		return ConnectedMsg{Client: client, Host: host}
	}
}

type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

// ConnectingView is shown while SSH connection is established
type ConnectingView struct {
	host sshpkg.Host
	done bool
	err  error
}

func NewConnectingView(host sshpkg.Host) ConnectingView {
	return ConnectingView{host: host}
}

func (m ConnectingView) Init() tea.Cmd {
	return connectCmd(m.host)
}

func (m ConnectingView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ConnectedMsg:
		m.done = true
		return m, tea.Quit
	case errMsg:
		m.err = msg.err
		return m, tea.Quit
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m ConnectingView) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	if m.err != nil {
		return tea.NewView(fmt.Sprintf("Error: %v\n", m.err))
	}
	return tea.NewView(fmt.Sprintf("Connecting to %s...\n", m.host.Name))
}

func RunConnect(host sshpkg.Host) (*sshpkg.Client, error) {
	p := tea.NewProgram(NewConnectingView(host))
	m, err := p.Run()
	if err != nil {
		return nil, err
	}
	cv := m.(ConnectingView)
	if cv.err != nil {
		return nil, cv.err
	}
	return sshpkg.Connect(host)
}
