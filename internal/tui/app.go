package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/beagle/drivesync/internal/clone"
	"github.com/beagle/drivesync/internal/config"
	"github.com/beagle/drivesync/internal/devices"
)

// Screen represents the current screen in the TUI.
type Screen int

const (
	ScreenLoading Screen = iota
	ScreenSelfOverwrite   // Pre-configured self-overwrite mode
	ScreenSelectSource
	ScreenSelectDest
	ScreenSizeAnalysis
	ScreenConfirm
	ScreenProgress
	ScreenComplete
	ScreenError
)

// Model is the main Bubble Tea model.
type Model struct {
	// Configuration
	config     *config.Config
	configPath string

	// State
	screen     Screen
	disks      []*devices.Disk
	cursor     int
	sourceDisk *devices.Disk
	destDisk   *devices.Disk
	analysis   *clone.SizeAnalysis

	// Progress
	progress     clone.Progress
	cloneResult  *clone.Result
	progressChan chan clone.Progress

	// Confirmation
	confirmInput string

	// Self-overwrite mode
	selfOverwrite bool
	bootDevice    *devices.Disk

	// Error
	errorMsg string

	// Display
	width  int
	height int
	styles Styles

	// For testing
	quitting bool
}

// NewModel creates a new TUI model with default configuration.
func NewModel() Model {
	return NewModelWithConfig(config.DefaultConfig(), "")
}

// NewModelWithConfig creates a new TUI model with the given configuration.
func NewModelWithConfig(cfg *config.Config, configPath string) Model {
	return Model{
		config:     cfg,
		configPath: configPath,
		screen:     ScreenLoading,
		styles:     DefaultStyles(),
		width:      80,
		height:     24,
	}
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return m.loadDisks
}

// loadDisks discovers available disks and processes configuration.
func (m Model) loadDisks() tea.Msg {
	disks, err := devices.Enumerate()
	if err != nil {
		return errMsg{err}
	}
	return disksLoadedMsg{disks}
}

// Messages
type disksLoadedMsg struct {
	disks []*devices.Disk
}

type errMsg struct {
	err error
}

type progressMsg struct {
	progress clone.Progress
}

type cloneCompleteMsg struct {
	result *clone.Result
	err    error
}

type autoStartMsg struct{}

// Update handles messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case disksLoadedMsg:
		m.disks = msg.disks
		return m.processConfig()

	case errMsg:
		m.screen = ScreenError
		m.errorMsg = msg.err.Error()
		return m, nil

	case progressMsg:
		m.progress = msg.progress
		return m, m.waitForProgress()

	case cloneCompleteMsg:
		if msg.err != nil {
			m.screen = ScreenError
			m.errorMsg = msg.err.Error()
		} else {
			m.screen = ScreenComplete
			m.cloneResult = msg.result
		}
		return m, m.handleComplete()

	case autoStartMsg:
		return m.startClone()
	}

	return m, nil
}

// processConfig processes the configuration after disks are loaded.
func (m Model) processConfig() (tea.Model, tea.Cmd) {
	// Detect boot device
	ctx, _ := devices.DetectBootContext(m.disks)
	if ctx != nil {
		m.bootDevice = ctx.BootDevice
	}

	// If config has pre-configured drives, try to match them
	if m.config.IsPreConfigured() {
		srcSpec := &devices.DriveSpec{
			Type:  m.config.Source.Type,
			Value: m.config.Source.Value,
		}
		dstSpec := &devices.DriveSpec{
			Type:  m.config.Destination.Type,
			Value: m.config.Destination.Value,
		}

		result, _ := devices.MatchDrivesFromConfig(m.disks, srcSpec, dstSpec)

		if result.Error != "" {
			m.screen = ScreenError
			m.errorMsg = result.Error
			return m, nil
		}

		m.sourceDisk = result.Source
		m.destDisk = result.Destination
		m.selfOverwrite = m.config.IsSelfOverwrite()

		// Analyze size
		m.analyzeSize()

		if !m.analysis.CanClone {
			m.screen = ScreenError
			m.errorMsg = m.analysis.ErrorMessage
			return m, nil
		}

		// Handle different modes
		switch m.config.Mode {
		case config.ModeAuto:
			m.screen = ScreenProgress
			return m, func() tea.Msg { return autoStartMsg{} }

		case config.ModeConfirm:
			m.screen = ScreenSelfOverwrite
			return m, nil

		default: // ModeInteractive
			m.screen = ScreenSelfOverwrite
			return m, nil
		}
	}

	// No pre-configured drives, check for self-overwrite detection
	if ctx != nil && ctx.CanSelfOverwrite {
		m.selfOverwrite = true
		m.bootDevice = ctx.BootDevice
		m.sourceDisk = ctx.InternalDrive
		m.destDisk = ctx.BootDevice
		m.analyzeSize()

		if m.analysis.CanClone {
			m.screen = ScreenSelfOverwrite
		} else {
			m.screen = ScreenError
			m.errorMsg = m.analysis.ErrorMessage
		}
		return m, nil
	}

	// Standard interactive mode
	m.screen = ScreenSelectSource
	return m, nil
}

// handleKey processes keyboard input.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		if m.screen != ScreenProgress {
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil

	case "up", "k":
		if m.screen == ScreenSelectSource || m.screen == ScreenSelectDest {
			if m.cursor > 0 {
				m.cursor--
			}
		}
		return m, nil

	case "down", "j":
		if m.screen == ScreenSelectSource || m.screen == ScreenSelectDest {
			if m.cursor < len(m.disks)-1 {
				m.cursor++
			}
		}
		return m, nil

	case "enter":
		return m.handleEnter()

	case "esc":
		return m.handleEscape()

	case "m", "M":
		// Switch to manual mode from self-overwrite screen
		if m.screen == ScreenSelfOverwrite {
			m.selfOverwrite = false
			m.sourceDisk = nil
			m.destDisk = nil
			m.screen = ScreenSelectSource
			m.cursor = 0
		}
		return m, nil

	case "r", "R":
		if m.screen == ScreenComplete {
			return m, m.reboot()
		}
		return m, nil

	case "p", "P":
		if m.screen == ScreenComplete {
			return m, m.powerOff()
		}
		return m, nil

	case "backspace":
		if m.screen == ScreenConfirm && len(m.confirmInput) > 0 {
			m.confirmInput = m.confirmInput[:len(m.confirmInput)-1]
		}
		return m, nil

	default:
		// Handle text input for confirmation
		if m.screen == ScreenConfirm && len(msg.String()) == 1 {
			m.confirmInput += msg.String()
			if m.confirmInput == "clone" {
				return m.startClone()
			}
		}
		return m, nil
	}
}

// handleEnter processes Enter key presses.
func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenSelfOverwrite:
		// Start clone from self-overwrite screen
		return m.startClone()

	case ScreenSelectSource:
		if len(m.disks) > 0 && m.cursor < len(m.disks) {
			m.sourceDisk = m.disks[m.cursor]
			m.screen = ScreenSelectDest
			m.cursor = 0
		}

	case ScreenSelectDest:
		if len(m.disks) > 0 && m.cursor < len(m.disks) {
			selected := m.disks[m.cursor]
			if selected.Path != m.sourceDisk.Path {
				m.destDisk = selected
				m.analyzeSize()
				if m.analysis.CanClone {
					if m.analysis.NeedsGPTFixup {
						m.screen = ScreenSizeAnalysis
					} else {
						m.screen = ScreenConfirm
					}
				} else {
					m.screen = ScreenError
					m.errorMsg = m.analysis.ErrorMessage
				}
			}
		}

	case ScreenSizeAnalysis:
		m.screen = ScreenConfirm

	case ScreenComplete:
		// "Enter New Clone" - restart
		m = NewModelWithConfig(m.config, m.configPath)
		return m, m.loadDisks
	}

	return m, nil
}

// handleEscape processes Escape key presses.
func (m Model) handleEscape() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenSelfOverwrite:
		// Go to manual mode
		m.selfOverwrite = false
		m.sourceDisk = nil
		m.destDisk = nil
		m.screen = ScreenSelectSource
		m.cursor = 0

	case ScreenSelectDest:
		m.screen = ScreenSelectSource
		m.sourceDisk = nil
		m.cursor = 0

	case ScreenSizeAnalysis:
		m.screen = ScreenSelectDest

	case ScreenConfirm:
		m.confirmInput = ""
		if m.analysis != nil && m.analysis.NeedsGPTFixup {
			m.screen = ScreenSizeAnalysis
		} else {
			m.screen = ScreenSelectDest
		}

	case ScreenError:
		m = NewModelWithConfig(m.config, m.configPath)
		return m, m.loadDisks
	}

	return m, nil
}

// analyzeSize performs size analysis for the clone operation.
func (m *Model) analyzeSize() {
	srcSize := m.sourceDisk.SizeBytes
	dstSize := m.destDisk.SizeBytes

	// TODO: Get last used LBA from GPT
	srcLastUsed := int64(0)

	analysis := clone.Analyze(srcSize, dstSize, srcLastUsed)
	m.analysis = &analysis
}

// startClone begins the clone operation.
func (m Model) startClone() (tea.Model, tea.Cmd) {
	m.screen = ScreenProgress
	m.progressChan = make(chan clone.Progress, 10)

	return m, tea.Batch(
		m.runClone(),
		m.waitForProgress(),
	)
}

// runClone executes the clone operation in a goroutine.
func (m Model) runClone() tea.Cmd {
	return func() tea.Msg {
		opts := clone.DefaultOptions()
		if m.analysis != nil {
			opts.CloneBytes = m.analysis.CloneBytes
			opts.FixupGPT = m.analysis.NeedsGPTFixup
			opts.DestSize = m.destDisk.SizeBytes
		}
		if m.config != nil {
			opts.Verify = m.config.Verify
			if m.config.BlockSize > 0 {
				opts.BlockSize = m.config.BlockSize
			}
			opts.DirectIO = m.config.DirectIO
		}

		result, err := clone.Clone(m.sourceDisk.Path, m.destDisk.Path, opts, m.progressChan)
		close(m.progressChan)

		return cloneCompleteMsg{result: result, err: err}
	}
}

// waitForProgress waits for progress updates.
func (m Model) waitForProgress() tea.Cmd {
	return func() tea.Msg {
		if m.progressChan == nil {
			return nil
		}
		progress, ok := <-m.progressChan
		if !ok {
			return nil
		}
		return progressMsg{progress: progress}
	}
}

// handleComplete handles post-clone actions based on config.
func (m Model) handleComplete() tea.Cmd {
	if m.config == nil {
		return nil
	}

	switch m.config.OnComplete {
	case config.OnCompleteShutdown:
		return m.powerOff()
	case config.OnCompleteReboot:
		return m.reboot()
	default:
		return nil
	}
}

// reboot initiates a system reboot.
func (m Model) reboot() tea.Cmd {
	return func() tea.Msg {
		// In production, this would call syscall.Reboot
		// For now, just quit
		return tea.Quit()
	}
}

// powerOff initiates a system shutdown.
func (m Model) powerOff() tea.Cmd {
	return func() tea.Msg {
		// In production, this would call syscall.Reboot with LINUX_REBOOT_CMD_POWER_OFF
		// For now, just quit
		return tea.Quit()
	}
}

// View renders the TUI.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var content string

	switch m.screen {
	case ScreenLoading:
		content = m.viewLoading()
	case ScreenSelfOverwrite:
		content = m.viewSelfOverwrite()
	case ScreenSelectSource:
		content = m.viewSelectSource()
	case ScreenSelectDest:
		content = m.viewSelectDest()
	case ScreenSizeAnalysis:
		content = m.viewSizeAnalysis()
	case ScreenConfirm:
		content = m.viewConfirm()
	case ScreenProgress:
		content = m.viewProgress()
	case ScreenComplete:
		content = m.viewComplete()
	case ScreenError:
		content = m.viewError()
	default:
		content = "Unknown screen"
	}

	return m.styles.Window.Render(content)
}

// viewLoading renders the loading screen.
func (m Model) viewLoading() string {
	title := m.styles.WindowTitle.Render("DriveSync")
	content := "\n\n  Detecting drives...\n"
	return title + content
}

// viewSelfOverwrite renders the self-overwrite confirmation screen.
func (m Model) viewSelfOverwrite() string {
	title := m.styles.WindowTitle.Render("DriveSync")

	var modeInfo string
	if m.configPath != "" {
		modeInfo = fmt.Sprintf("\n  Config: %s\n", m.configPath)
	}

	content := fmt.Sprintf(`%s
  Ready to clone

  FROM (internal):
    [%s] %s    %.1f GB

  TO (this drive):
    [%s] %s    %.1f GB
    └─ Will be completely overwritten

`,
		modeInfo,
		m.sourceDisk.Name,
		m.sourceDisk.DisplayName(),
		m.sourceDisk.SizeGB(),
		m.destDisk.Name,
		m.destDisk.DisplayName(),
		m.destDisk.SizeGB(),
	)

	// Size difference warning if applicable
	if m.analysis != nil && m.analysis.NeedsGPTFixup {
		content += m.styles.Info.Render(fmt.Sprintf("  Note: Destination is %.1f MB smaller. GPT will be adjusted.\n\n",
			float64(-m.analysis.Difference)/1e6))
	}

	button := m.styles.ButtonActive.Render(">>> Press Enter to Start Clone <<<")
	content += "  " + button + "\n\n"

	content += "  DriveSync will continue running from memory.\n"

	footer := m.renderFooter([]footerItem{
		{"Enter", "Clone"},
		{"M", "Manual Mode"},
		{"Q", "Quit"},
	})

	return title + content + "\n" + footer
}

// viewSelectSource renders the source drive selection screen.
func (m Model) viewSelectSource() string {
	title := m.styles.WindowTitle.Render("DriveSync")
	subtitle := "\n\n  Select SOURCE drive:\n\n"

	var diskList string
	for i, disk := range m.disks {
		diskList += m.renderDisk(disk, i == m.cursor) + "\n"
	}

	footer := m.renderFooter([]footerItem{
		{"↑/↓", "Select"},
		{"Enter", "Confirm"},
		{"Q", "Quit"},
	})

	return title + subtitle + diskList + "\n" + footer
}

// viewSelectDest renders the destination drive selection screen.
func (m Model) viewSelectDest() string {
	title := m.styles.WindowTitle.Render("DriveSync")
	subtitle := "\n\n  Select DESTINATION drive:\n\n"
	sourceInfo := fmt.Sprintf("  Source: %s\n\n", m.sourceDisk.DisplayName())

	var diskList string
	for i, disk := range m.disks {
		if disk.Path == m.sourceDisk.Path {
			continue
		}
		diskList += m.renderDisk(disk, i == m.cursor) + "\n"
	}

	footer := m.renderFooter([]footerItem{
		{"↑/↓", "Select"},
		{"Enter", "Confirm"},
		{"Esc", "Back"},
		{"Q", "Quit"},
	})

	return title + subtitle + sourceInfo + diskList + "\n" + footer
}

// viewSizeAnalysis renders the size analysis screen.
func (m Model) viewSizeAnalysis() string {
	title := m.styles.WindowTitle.Render("Size Analysis")

	content := fmt.Sprintf(`

  Source:      %s    %.1f GB
  Destination: %s    %.1f GB
  Difference:  %.1f MB (destination smaller)

`,
		m.sourceDisk.DisplayName(),
		m.sourceDisk.SizeGB(),
		m.destDisk.DisplayName(),
		m.destDisk.SizeGB(),
		float64(-m.analysis.Difference)/1e6,
	)

	if m.analysis.CanClone {
		content += m.styles.Success.Render("  ✓ Clone will fit. Trailing space becomes unallocated.")
	} else {
		content += m.styles.Error.Render("  ✗ " + m.analysis.ErrorMessage)
	}

	content += "\n"

	footer := m.renderFooter([]footerItem{
		{"Enter", "Continue"},
		{"Esc", "Cancel"},
	})

	return title + content + "\n" + footer
}

// viewConfirm renders the confirmation screen.
func (m Model) viewConfirm() string {
	title := m.styles.WindowTitle.Render("Confirm Clone")

	content := fmt.Sprintf(`

  SOURCE: %s (%.1f GB)
          %s

  DESTINATION: %s (%.1f GB)
               %s

`,
		m.sourceDisk.DisplayName(),
		m.sourceDisk.SizeGB(),
		m.sourceDisk.Path,
		m.destDisk.DisplayName(),
		m.destDisk.SizeGB(),
		m.destDisk.Path,
	)

	warning := m.styles.Warning.Render("  ⚠ ALL DATA ON DESTINATION WILL BE DESTROYED")
	content += warning + "\n\n"

	prompt := fmt.Sprintf(`  Type "clone" to confirm:  %s█`, m.confirmInput)
	content += prompt + "\n"

	footer := m.renderFooter([]footerItem{
		{"Esc", "Cancel"},
	})

	return title + content + "\n" + footer
}

// viewProgress renders the progress screen.
func (m Model) viewProgress() string {
	title := m.styles.WindowTitle.Render("Cloning")

	content := fmt.Sprintf("\n\n  %s  →  %s\n\n",
		m.sourceDisk.DisplayName(),
		m.destDisk.DisplayName(),
	)

	// Progress bar
	barWidth := 50
	filled := int(m.progress.Percent() / 100 * float64(barWidth))
	bar := ""
	for i := 0; i < barWidth; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	content += fmt.Sprintf("  %s  %.1f%%\n\n", bar, m.progress.Percent())

	// Stats
	content += fmt.Sprintf("  Copied:     %.1f GB / %.1f GB\n",
		float64(m.progress.Copied)/1e9,
		float64(m.progress.Total)/1e9,
	)
	content += fmt.Sprintf("  Speed:      %.0f MB/s\n", m.progress.SpeedMBps())
	content += fmt.Sprintf("  Elapsed:    %s\n", formatDuration(m.progress.Elapsed))
	content += fmt.Sprintf("  Remaining:  ~%s\n", formatDuration(m.progress.Remaining))

	if m.selfOverwrite {
		content += "\n" + m.styles.Warning.Render("  Running from RAM - destination is being overwritten")
	}

	footer := m.renderFooter([]footerItem{
		{"Ctrl+C", "Abort"},
	})

	return title + content + "\n" + footer
}

// viewComplete renders the completion screen.
func (m Model) viewComplete() string {
	title := m.styles.WindowTitle.Render("Complete")

	content := "\n\n" + m.styles.Success.Render("  ✓ Clone completed successfully") + "\n\n"

	if m.cloneResult != nil {
		content += fmt.Sprintf("  Copied:     %.1f GB\n", float64(m.cloneResult.BytesCopied)/1e9)
		content += fmt.Sprintf("  Duration:   %s\n", formatDuration(m.cloneResult.Duration))
		content += fmt.Sprintf("  Avg Speed:  %.0f MB/s\n", m.cloneResult.AvgSpeedMBps())
		if m.cloneResult.Verified {
			if m.cloneResult.VerifyPassed {
				content += m.styles.Success.Render("  Verified:   ✓ All blocks match") + "\n"
			} else {
				content += m.styles.Error.Render("  Verified:   ✗ Verification failed") + "\n"
			}
		}
	}

	if m.selfOverwrite {
		content += "\n  The destination drive now contains your cloned system.\n"
		content += "  Shutdown, swap drives, and boot from the new drive.\n"
	} else {
		content += "\n  It is now safe to remove the destination drive.\n"
	}

	footer := m.renderFooter([]footerItem{
		{"Enter", "New Clone"},
		{"R", "Reboot"},
		{"P", "Power Off"},
	})

	return title + content + "\n" + footer
}

// viewError renders the error screen.
func (m Model) viewError() string {
	title := m.styles.WindowTitle.Render("Error")

	content := "\n\n" + m.styles.Error.Render("  ✗ "+m.errorMsg) + "\n"

	footer := m.renderFooter([]footerItem{
		{"Esc", "Back"},
		{"Q", "Quit"},
	})

	return title + content + "\n" + footer
}

// renderDisk renders a single disk entry.
func (m Model) renderDisk(disk *devices.Disk, selected bool) string {
	cursor := "  ○ "
	if selected {
		cursor = "  ● "
	}

	name := fmt.Sprintf("[%s] %s", disk.Name, disk.DisplayName())
	size := fmt.Sprintf("%.1f GB", disk.SizeGB())

	line := cursor + m.styles.DriveName.Render(name)
	line = lipgloss.JoinHorizontal(lipgloss.Top,
		line,
		"  ",
		m.styles.DriveSize.Render(size),
	)

	// Mark boot device
	if m.bootDevice != nil && disk.Path == m.bootDevice.Path {
		line += "  " + m.styles.Info.Render("[BOOT]")
	}

	// Add partition info
	for i, part := range disk.Partitions {
		tree := "    ├─ "
		if i == len(disk.Partitions)-1 {
			tree = "    └─ "
		}
		partLine := fmt.Sprintf("%s: %s", part.Name, part.TypeName)
		if part.FSType != "" {
			partLine += fmt.Sprintf(" (%s)", devices.NormalizeFSType(part.FSType))
		}
		partSize := fmt.Sprintf("%.1f GB", part.SizeGB())
		line += "\n" + m.styles.PartitionTree.Render(tree) +
			m.styles.Partition.Render(partLine) +
			"  " + m.styles.DriveSize.Render(partSize)
	}

	return line
}

// footerItem represents a keyboard shortcut in the footer.
type footerItem struct {
	key   string
	label string
}

// renderFooter renders the footer with keyboard shortcuts.
func (m Model) renderFooter(items []footerItem) string {
	var parts []string
	for _, item := range items {
		parts = append(parts,
			m.styles.FooterKey.Render(item.key)+" "+
				m.styles.Footer.Render(item.label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// formatDuration formats a duration for display.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// Run starts the TUI application with default configuration.
func Run() error {
	return RunWithConfig(nil, "")
}

// RunWithConfig starts the TUI application with the given configuration.
func RunWithConfig(cfg *config.Config, configPath string) error {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	m := NewModelWithConfig(cfg, configPath)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
