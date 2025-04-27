// Copyright (C) 2025 Alex Katlein
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package borg

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"os/exec"
)

type Action interface {
	Id() string
	Execute(self Action) error

	SetId(id string)
}

type actionJob struct {
	action Action
}

func (a actionJob) Run() {
	_ = a.action.Execute(a.action)
}

func Wrap(a Action) cron.Job {
	return actionJob{a}
}

type closureAction struct {
	id     string
	action func(self Action) error
}

func (c *closureAction) Id() string {
	return c.id
}

func (c *closureAction) Execute(self Action) error {
	log.Info().Str("actionId", c.id).Msg("executing closure action")

	err := c.action(self)
	if err != nil {
		log.Warn().
			Err(err).
			Str("actionId", c.id).
			Msg("Failed to execute action")
	} else {
		log.Info().Str("actionId", c.id).Msg("closure action executed successfully")
	}

	return err
}

func (c *closureAction) SetId(id string) {
	c.id = id
}

type execAction struct {
	id      string
	command []string
}

func (e *execAction) Id() string {
	return e.id
}

func (e *execAction) Execute(self Action) error {
	var stderr bytes.Buffer

	log.Info().
		Str("actionId", self.Id()).
		Strs("command", e.command).
		Msgf("executing %v", e.command[0])

	cmd := exec.Command(e.command[0], e.command[1:]...)
	cmd.Stderr = &stderr

	stdout, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			log.Warn().
				Str("actionId", self.Id()).
				Strs("command", e.command).
				Int("exit-code", exitErr.ExitCode()).
				Str("stderr", stderr.String()).
				Msg("Command exited with non-zero code")
		} else {
			log.Warn().
				Err(err).
				Str("actionId", self.Id()).
				Strs("command", e.command).
				Msg("Error executing command")
		}

		return err
	}

	log.Info().
		Str("actionId", self.Id()).
		Strs("command", e.command).
		Int("exit-code", cmd.ProcessState.ExitCode()).
		Str("stdout", string(stdout)).
		Msg("Command executed successfully")

	return nil
}

func (e *execAction) SetId(id string) {
	e.id = id
}

func NewExecAction(command []string) Action {
	return &execAction{
		id:      rand.Text(),
		command: command,
	}
}

type SequenceAction struct {
	id      string
	actions []Action
}

func (s *SequenceAction) Id() string {
	return s.id
}

func (s *SequenceAction) Execute(self Action) error {
	if len(s.actions) == 0 {
		return fmt.Errorf("sequence action %s has no actions", s.id)
	}

	for _, action := range s.actions {
		err := action.Execute(self)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *SequenceAction) SetId(id string) {
	s.id = id
	for _, action := range s.actions {
		action.SetId(id)
	}
}

func NewSequenceAction() *SequenceAction {
	return &SequenceAction{
		id:      rand.Text(),
		actions: make([]Action, 5),
	}
}

func (s *SequenceAction) Push(action Action) *SequenceAction {
	s.actions = append(s.actions, action)
	return s
}

type ComposedAction struct {
	id             string
	delegate       Action
	preActions     []Action
	postActions    []Action
	finallyActions []Action
}

func (c *ComposedAction) Id() string {
	return c.id
}

func (c *ComposedAction) Execute(self Action) error {
	log.Info().Str("actionId", c.id).Msg("executing composed action")

	if len(c.preActions) > 0 {
		log.Info().Str("actionId", c.id).Msg("executing pre actions")
	}

	var err error
	for _, preAction := range c.preActions {
		err = preAction.Execute(self)
		if err != nil {
			break
		}
	}

	if err == nil {
		log.Info().Str("actionId", c.id).Msg("executing action delegate")
		err = c.delegate.Execute(self)
	}

	if err == nil {
		if len(c.postActions) > 0 {
			log.Info().Str("actionId", c.id).Msg("executing post actions")
		}

		for _, postAction := range c.postActions {
			err = postAction.Execute(self)
			if err != nil {
				break
			}
		}
	}

	if len(c.finallyActions) > 0 {
		log.Info().Str("actionId", c.id).Msg("executing finally actions")
	}

	for _, finallyAction := range c.finallyActions {
		fErr := finallyAction.Execute(self)
		if fErr != nil {
			log.Debug().
				Err(fErr).
				Str("actionId", c.id).
				Msgf("Error executing finally action in composed action")
		}
	}

	if err != nil {
		log.Warn().
			Err(err).
			Str("actionId", c.id).
			Msg("failed to execute composed action")
	}

	return err
}

func (c *ComposedAction) SetId(id string) {
	c.id = id
	c.delegate.SetId(id)
	for _, action := range c.preActions {
		action.SetId(id)
	}
	for _, action := range c.postActions {
		action.SetId(id)
	}
	for _, finallyAction := range c.finallyActions {
		finallyAction.SetId(id)
	}
}

func NewComposedAction(delegate Action) *ComposedAction {
	id := rand.Text()
	delegate.SetId(id)

	return &ComposedAction{
		id:       id,
		delegate: delegate,
	}
}

func (c *ComposedAction) Pre(action Action) *ComposedAction {
	action.SetId(c.id)
	c.preActions = append(c.preActions, action)
	return c
}

func (c *ComposedAction) Post(action Action) *ComposedAction {
	action.SetId(c.id)
	c.postActions = append(c.postActions, action)
	return c
}

func (c *ComposedAction) Finally(action Action) *ComposedAction {
	action.SetId(c.id)
	c.finallyActions = append(c.finallyActions, action)
	return c
}
