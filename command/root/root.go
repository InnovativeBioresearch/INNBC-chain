package root

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	/* START ALPHA CHANGE 3265 -- remove the legacy RLP backup command import -- */
	/* FINISH ALPHA CHANGE 3265 -- remove the legacy RLP backup command import -- */
	/* START ALPHA CHANGE 334 -- prune legacy Polygon bridge and rootchain command imports -- */
	"github.com/innovativebioresearch/innbc-chain/command/genesis"
	"github.com/innovativebioresearch/innbc-chain/command/helper"

	/* START ALPHA CHANGE 713 -- remove legacy ibft command import from root CLI -- */
	/* FINISH ALPHA CHANGE 713 -- remove legacy ibft command import from root CLI -- */
	/* START ALPHA CHANGE 3277 -- remove the empty INNBCBFT CLI wrapper import -- */
	/* FINISH ALPHA CHANGE 3277 -- remove the empty INNBCBFT CLI wrapper import -- */
	/* START ALPHA CHANGE 3253 -- remove the retired INNBCBFT identity command import -- */
	/* FINISH ALPHA CHANGE 3253 -- remove the retired INNBCBFT identity command import -- */
	"github.com/innovativebioresearch/innbc-chain/command/license"
	"github.com/innovativebioresearch/innbc-chain/command/monitor"

	/* START ALPHA CHANGE 754 -- add node service management command import -- */
	"github.com/innovativebioresearch/innbc-chain/command/node"
	/* FINISH ALPHA CHANGE 754 -- add node service management command import -- */
	"github.com/innovativebioresearch/innbc-chain/command/peerdirectory"
	"github.com/innovativebioresearch/innbc-chain/command/peers"
	"github.com/innovativebioresearch/innbc-chain/command/regenesis"

	/* FINISH ALPHA CHANGE 334 -- prune legacy Polygon bridge and rootchain command imports -- */
	/* START ALPHA CHANGE 3274 -- remove the retired legacy secrets CLI import -- */
	/* FINISH ALPHA CHANGE 3274 -- remove the retired legacy secrets CLI import -- */
	"github.com/innovativebioresearch/innbc-chain/command/server"

	/* START ALPHA CHANGE 1197 -- add manual snapshot command import -- */
	"github.com/innovativebioresearch/innbc-chain/command/snapshot"
	/* FINISH ALPHA CHANGE 1197 -- add manual snapshot command import -- */
	"github.com/innovativebioresearch/innbc-chain/command/status"
	"github.com/innovativebioresearch/innbc-chain/command/txpool"
	"github.com/innovativebioresearch/innbc-chain/command/version"
)

type RootCommand struct {
	baseCmd *cobra.Command
}

func NewRootCommand() *RootCommand {
	rootCommand := &RootCommand{
		baseCmd: &cobra.Command{
			/*START ALPHA CHANGE 4 -- rebrand top-level CLI short description --*/
			Short: "INNBC Chain is a sovereign EVM-compatible blockchain client built on a customized Polygon Edge foundation",
			/*FINISH ALPHA CHANGE 4 -- rebrand top-level CLI short description --*/
		},
	}

	helper.RegisterJSONOutputFlag(rootCommand.baseCmd)

	rootCommand.registerSubCommands()

	return rootCommand
}

func (rc *RootCommand) registerSubCommands() {
	rc.baseCmd.AddCommand(
		version.GetCommand(),
		txpool.GetCommand(),
		status.GetCommand(),
		/* START ALPHA CHANGE 3274 -- retire the legacy secrets command group -- */
		/* FINISH ALPHA CHANGE 3274 -- retire the legacy secrets command group -- */
		peers.GetCommand(),
		/* START ALPHA CHANGE 335 -- remove legacy rootchain command from top-level CLI -- */
		/* FINISH ALPHA CHANGE 335 -- remove legacy rootchain command from top-level CLI -- */
		monitor.GetCommand(),
		/* START ALPHA CHANGE 755 -- expose installed node service management CLI -- */
		node.GetCommand(),
		/* FINISH ALPHA CHANGE 755 -- expose installed node service management CLI -- */
		peerdirectory.GetCommand(),
		/* START ALPHA CHANGE 714 -- remove legacy ibft command from root CLI -- */
		/* FINISH ALPHA CHANGE 714 -- remove legacy ibft command from root CLI -- */
		/* START ALPHA CHANGE 3265 -- retire the top-level legacy RLP backup command -- */
		/* FINISH ALPHA CHANGE 3265 -- retire the top-level legacy RLP backup command -- */
		genesis.GetCommand(),
		server.GetCommand(),
		/* START ALPHA CHANGE 1198 -- register manual snapshot command -- */
		snapshot.GetCommand(),
		/* FINISH ALPHA CHANGE 1198 -- register manual snapshot command -- */
		license.GetCommand(),
		/* START ALPHA CHANGE 3253 -- unregister the retired INNBCBFT identity command -- */
		/* FINISH ALPHA CHANGE 3253 -- unregister the retired INNBCBFT identity command -- */
		/* START ALPHA CHANGE 3277 -- retire the empty INNBCBFT CLI wrapper -- */
		/* FINISH ALPHA CHANGE 3277 -- retire the empty INNBCBFT CLI wrapper -- */
		/* START ALPHA CHANGE 336 -- remove legacy bridge command from top-level CLI -- */
		/* FINISH ALPHA CHANGE 336 -- remove legacy bridge command from top-level CLI -- */
		regenesis.GetCommand(),
	)
}

func (rc *RootCommand) Execute() {
	if err := rc.baseCmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		os.Exit(1)
	}
}
