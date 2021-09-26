package main

import (
	"fmt"
	"image"
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func main() {
	go func() {
		w := app.NewWindow()
		if err := loop(w); err != nil {
			log.Fatal(err)
		}

		os.Exit(0)
	}()

	app.Main()
}

var (
	btn1 = &widget.Clickable{}
	btn2 = &widget.Clickable{}
)

func loop(w *app.Window) error {
	th := material.NewTheme(gofont.Collection())

	var ops op.Ops

	for {
		e := <-w.Events()
		switch e := e.(type) {
		case system.DestroyEvent:
			return e.Err
		case system.FrameEvent:
			gtx := layout.NewContext(&ops, e)

			layout.UniformInset(unit.Dp(25)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{
					Axis:      layout.Vertical,
					Spacing:   layout.SpaceEvenly,
					Alignment: layout.Start,
				}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					for btn1.Clicked() {
						fmt.Println("btn1 click")
					}

					gtx.Constraints = layout.Exact(image.Pt(100, 40))
					l := material.Button(th, btn1, "lo")
					return l.Layout(gtx)
				}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					for btn2.Clicked() {
						fmt.Println("btn2 click")
					}

					gtx.Constraints = layout.Exact(image.Pt(100, 40))
					l := material.Button(th, btn2, "lo2")
					return l.Layout(gtx)
				}))
			})

			e.Frame(gtx.Ops)
		}
	}
}
