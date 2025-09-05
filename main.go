package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// struct holds process info gathered by getProcessInfo
type ProcessInfo struct {
	PID     int32
	Name    string
	CPU     float64
	Memory  float64
	MemSize uint64
}

// function gathers all process info
func getProcessInfo() []ProcessInfo {
	processes, _ := process.Processes()
	var processInfo []ProcessInfo

	for _, p := range processes {
		name, _ := p.Name()
		cpu, _ := p.CPUPercent()
		mem, _ := p.MemoryPercent()
		memInfo, _ := p.MemoryInfo()

		if memInfo != nil {
			processInfo = append(processInfo, ProcessInfo{
				PID:     p.Pid,
				Name:    name,
				CPU:     cpu,
				Memory:  float64(mem),
				MemSize: memInfo.RSS,
			})
		}
	}

	return processInfo
}

// clear screen, set screen size variables
func updateScreen(screen tcell.Screen) {
	screen.Clear()
	w, h := screen.Size()

	//calc memUsed percentage
	v, _ := mem.VirtualMemory()
	memUsed := float64(v.Used) / float64(v.Total) * 100

	//draw memory bar
	drawBar(screen, 1, 1, w-2, "Memory", memUsed,
		fmt.Sprintf("Total: %.2fGB Used: %.2fGB Free: %.2fGB (%.1f%%)",
			float64(v.Total)/1024/1024/1024,
			float64(v.Used)/1024/1024/1024,
			float64(v.Free)/1024/1024/1024,
			memUsed))

	//cpu usage display
	c, _ := cpu.Percent(0, false)
	cpuUsed := c[0]

	drawBar(screen, 1, 3, w-2, "CPU", cpuUsed,
		fmt.Sprintf("Usage: (%.1f%%)", cpuUsed))

	//network stats
	n, _ := net.IOCounters(false)
	netStats := fmt.Sprintf("Recieved: %.2fMB (%d pkts) Sent: %.2fMB) (%d pkts)",
		float64(n[0].BytesRecv)/1024/1024,
		n[0].PacketsRecv,
		float64(n[0].BytesSent)/1024/1024,
		n[0].PacketsSent)

	drawText(screen, 1, 5, netStats,
		tcell.StyleDefault.Foreground(tcell.ColorBlue))

	processes := getProcessInfo()
	drawProcessTable(screen, 1, 7, w/2-1, h-7, "Top Memory Usage", sortMemory(processes))
	drawProcessTable(screen, w/2+1, 7, w/2-2, h-7, "Top CPU Usage", sortCPU(processes))

	screen.Show()
}

// function will sort so top resourse using processes are shown
func sortMemory(processes []ProcessInfo) []ProcessInfo {
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].MemSize > processes[j].MemSize
	})
	if len(processes) > 10 {
		processes = processes[:10]
	}
	return processes
}

func sortCPU(processes []ProcessInfo) []ProcessInfo {
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].CPU > processes[j].CPU
	})
	if len(processes) > 10 {
		processes = processes[:10]
	}
	return processes
}

// clear screen, set screen size variables

// function for text
func drawText(screen tcell.Screen, x, y int, text string, style tcell.Style) {
	for i, r := range text {
		screen.SetContent(x+i, y, r, nil, style)
	}
}

// function for bars
func drawBar(screen tcell.Screen, x, y, w int, label string, value float64, stats string) {
	barWidth := w - 2
	filled := int(float64(barWidth) * value / 100)

	// label and stats of bars
	drawText(screen, x, y, label+": "+stats,
		tcell.StyleDefault.Foreground(tcell.ColorYellow))

	//styling
	screen.SetContent(x, y+1, '[', nil, tcell.StyleDefault)
	for i := 0; i < barWidth; i++ {
		char := ' '
		style := tcell.StyleDefault.Background(tcell.ColorDarkGray)
		if i < filled {
			style = tcell.StyleDefault.Background(tcell.ColorBlue)
		}
		screen.SetContent(x+1+i, y+1, char, nil, style)
	}
	screen.SetContent(x+barWidth+1, y+1, ']', nil, tcell.StyleDefault)
}

func drawProcessTable(screen tcell.Screen, x, y, _, h int, title string, processes []ProcessInfo) {
	drawText(screen, x, y, title,
		tcell.StyleDefault.Foreground(tcell.ColorYellow))

	drawText(screen, x, y+1, fmt.Sprintf("%-6s %-20s %-10s %-10s",
		"PID", "Name", "CPU%", "Memory"),
		tcell.StyleDefault.Foreground(tcell.ColorGreen))

	for i, p := range processes {
		if i >= h-3 {
			break
		}
		drawText(screen, x, y+2+i, fmt.Sprintf("%-6d %-20s %-10.1f %-10.1f",
			p.PID, truncateString(p.Name, 20), p.CPU, p.Memory),
			tcell.StyleDefault)
	}
}

func truncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length-3] + "..."
}

func main() {
	//start
	screen, err := tcell.NewScreen()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	if err := screen.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	defer screen.Fini()

	screen.SetStyle(tcell.StyleDefault.
		Background(tcell.ColorBlack).
		Foreground(tcell.ColorWhite))
	screen.Clear()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	//wait groups
	var wg sync.WaitGroup //starts WG counter at 1
	wg.Add(1)
	go func() {
		defer wg.Done() //will take away 1 from counter once go routine complete
		for {
			select { //watches ctx and signal channel
			case <-ctx.Done(): //stop go routine if process is done
				return
			case <-sigChan: //stops go routine if signal recieved, has user pressed ctrl+c?
				cancel()
				wg.Wait()
				return
			default:
				updateScreen(screen)
				time.Sleep(1 * time.Second) //else nothing detected then updates every second

			}
		}
	}()
	//if esc pressed

	for {
		switch ev := screen.PollEvent().(type) {
		case *tcell.EventKey:
			if ev.Key() == tcell.KeyEscape {
				cancel()
				return
			}
		case *tcell.EventResize:
			screen.Sync()
		}
	}

}
