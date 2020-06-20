// Copyright (c) 2013 Uwe Hoffmann. All rights reserved.

/*
Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google Inc. nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

package service

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/gonuts/flag"
	"github.com/uwedeportivo/commander"
	"github.com/uwedeportivo/romba/config"
)

type splitState struct {
	inQuotedZone bool
	previousRne  rune
}

func (st *splitState) shouldSplit(rne rune) bool {
	if rne == '\'' && st.previousRne != '\\' {
		st.inQuotedZone = !st.inQuotedZone
	}

	if rne == '=' && !st.inQuotedZone {
		rne = ' '
	}

	if unicode.IsSpace(rne) && st.previousRne != '\\' && !st.inQuotedZone {
		st.previousRne = rne
		return true
	}

	st.previousRne = rne
	return false
}

func splitIntoArgs(argLine string) ([]string, error) {
	st := new(splitState)

	n := 0
	inField := false

	for _, rne := range argLine {
		wasInField := inField
		inField = !st.shouldSplit(rne)
		if inField && !wasInField {
			n++
		}
	}

	a := make([]string, n)

	st.inQuotedZone = false
	st.previousRne = 0

	na := 0
	fieldStart := -1

	for i, rne := range argLine {
		if st.shouldSplit(rne) {
			if fieldStart >= 0 {
				a[na] = strings.Trim(argLine[fieldStart:i], "'")
				na++
				fieldStart = -1
			}
		} else if fieldStart == -1 {
			fieldStart = i
		}
	}

	if fieldStart >= 0 {
		a[na] = strings.Trim(argLine[fieldStart:], "'")
	}

	if st.inQuotedZone {
		return nil, fmt.Errorf("open quotes in %s", argLine)
	}
	return a, nil
}

func newCommand(writer io.Writer, rs *RombaService) *commander.Command {
	cmd := new(commander.Command)
	cmd.UsageLine = "Romba"
	cmd.Subcommands = make([]*commander.Command, 19)
	cmd.Flag = *flag.NewFlagSet("romba", flag.ContinueOnError)
	cmd.Stdout = writer
	cmd.Stderr = writer

	cmd.Subcommands[0] = &commander.Command{
		Run:       rs.startRefreshDats,
		UsageLine: "refresh-dats",
		Short:     "Refreshes the DAT index from the files in the DAT master directory tree.",
		Long: `
Refreshes the DAT index from the files in the DAT master directory tree.
Detects any changes in the DAT master directory tree and updates the DAT index
accordingly, marking deleted or overwritten dats as orphaned and updating
contents of any changed dats.`,
		Flag:   *flag.NewFlagSet("romba-refresh-dats", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[0].Flag.Int("workers", config.GlobalConfig.General.Workers,
		"how many workers to launch for the job")
	cmd.Subcommands[0].Flag.String("missingSha1s", "", "write paths of dats with missing sha1s into this file")

	cmd.Subcommands[1] = &commander.Command{
		Run:       rs.startArchive,
		UsageLine: "archive [-only-needed] [-include-zips] [-resume resumelog] <space-separated list of directories of ROM files>",
		Short:     "Adds ROM files from the specified directories to the ROM archive.",
		Long: `
Adds ROM files from the specified directories to the ROM archive.
Traverses the specified directory trees looking for zip files and normal files.
Unpacked files will be stored as individual entries. Prior to unpacking a zip
file, the external SHA1 is checked against the DAT index. 
If -only-needed is set, only those files are put in the ROM archive that
have a current entry in the DAT index.`,

		Flag:   *flag.NewFlagSet("romba-archive", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[1].Flag.Bool("only-needed", false, "only archive ROM files actually referenced by DAT files from the DAT index")
	cmd.Subcommands[1].Flag.String("resume", "", "resume a previously interrupted archive operation from the specified path")
	cmd.Subcommands[1].Flag.Int("include-zips", 0, "flag value == 1 means: add zip files themselves into the depot in addition"+
		" to their contents, flag value > 1 means add zip files themselves but don't add content")
	cmd.Subcommands[1].Flag.Int("workers", config.GlobalConfig.General.Workers,
		"how many workers to launch for the job")
	cmd.Subcommands[1].Flag.Int("include-gzips", 0, "flag value == 1 means: add gzip files themselves into the depot in addition"+
		" to their contents, flag value > 1 means add gzip files themselves but don't add content")
	cmd.Subcommands[1].Flag.Int("include-7zips", 0, "flag value == 1 means: add 7zip files themselves into the depot in addition"+
		" to their contents, flag value > 1 means add 7zip files themselves but don't add content")
	cmd.Subcommands[1].Flag.Bool("skip-initial-scan", false, "skip the initial scan of the files to determine amount of work")
	cmd.Subcommands[1].Flag.Bool("use-golang-zip", false, "use go zip implementation instead of zlib")
	cmd.Subcommands[1].Flag.Bool("no-db", false, "archive into depot but do not touch DB index and ignore only-needed flag")

	cmd.Subcommands[2] = &commander.Command{
		Run:       rs.purge,
		UsageLine: "purge-backup -backup <backupdir>",
		Short:     "Moves DAT index entries for orphaned DATs.",
		Long: `
Deletes DAT index entries for orphaned DATs and moves ROM files that are no
longer associated with any current DATs to the specified backup folder.
The files will be placed in the backup location using
a folder structure according to the original DAT master directory tree
structure. It also deletes the specified DATs from the DAT index.`,
		Flag:   *flag.NewFlagSet("romba-purge-backup", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[2].Flag.String("backup", "", "backup directory where backup files are moved to")
	cmd.Subcommands[2].Flag.Int("workers", config.GlobalConfig.General.Workers,
		"how many workers to launch for the job")
	cmd.Subcommands[2].Flag.String("depot", "", "work only on specified depot path")
	cmd.Subcommands[2].Flag.String("dats", "", "purge only roms declared in these dats")

	cmd.Subcommands[3] = &commander.Command{
		Run:       rs.dir2dat,
		UsageLine: "dir2dat -out <outputfile> -source <sourcedir>",
		Short:     "Creates a DAT file for the specified input directory and saves it to the -out filename.",
		Long: `
Walks the specified input directory and builds a DAT file that mirrors its
structure. Saves this DAT file in specified output filename.`,
		Flag:   *flag.NewFlagSet("romba-dir2dat", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[3].Flag.String("out", "", "output file")
	cmd.Subcommands[3].Flag.String("source", "", "source directory")
	cmd.Subcommands[3].Flag.String("name", "untitled", "name value in DAT header")
	cmd.Subcommands[3].Flag.String("description", "", "description value in DAT header")

	cmd.Subcommands[4] = &commander.Command{
		Run:       rs.diffdat,
		UsageLine: "diffdat -old <datfile> -new <datfile> -out <outputfile>",
		Short:     "Creates a DAT file with those entries that are in -new DAT.",
		Long: `
Creates a DAT file with those entries that are in -new DAT file and not
in -old DAT file. Ignores those entries in -old that are not in -new.`,
		Flag:   *flag.NewFlagSet("romba-diffdat", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[4].Flag.String("out", "", "output filename")
	cmd.Subcommands[4].Flag.String("old", "", "old DAT file")
	cmd.Subcommands[4].Flag.String("new", "", "new DAT file")
	cmd.Subcommands[4].Flag.String("name", "", "name for out DAT file")
	cmd.Subcommands[4].Flag.String("description", "", "description for out DAT file")

	cmd.Subcommands[5] = &commander.Command{
		Run:       rs.build,
		UsageLine: "build -out <outputdir> <list of DAT files or folders with DAT files>",
		Short:     "For each specified DAT file it creates the torrentzip files.",
		Long: `
For each specified DAT file it creates the torrentzip files in the specified
output dir. The files will be placed in the specified location using a folder
structure according to the original DAT master directory tree structure unless
the flag sha1Tree is used in which case the directory tree structure is the depot
sha1 directories.`,
		Flag:   *flag.NewFlagSet("romba-build", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[5].Flag.String("out", "", "output dir")
	cmd.Subcommands[5].Flag.Bool("fixdatOnly", false, "only fix dats and don't generate torrentzips")
	cmd.Subcommands[5].Flag.Bool("unzipAllGames", false, "don't generate torrentzips")
	cmd.Subcommands[5].Flag.Int("sha1Tree", 0, `if value >0 copy as sha1 tree. if value == 1,
keep compressed gzip, if value > 1 uncompress into destination sha1`)

	cmd.Subcommands[5].Flag.Int("workers", config.GlobalConfig.General.Workers,
		"how many workers to launch for the job")

	cmd.Subcommands[5].Flag.Int("subworkers", config.GlobalConfig.General.Workers,
		"how many subworkers to launch for each worker")

	cmd.Subcommands[6] = &commander.Command{
		Run:       rs.lookup,
		UsageLine: "lookup <list of hashes>",
		Short:     "For each specified hash it looks up any available information.",
		Long: `
For each specified hash it looks up any available information (dat or rom).`,
		Flag:   *flag.NewFlagSet("romba-lookup", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[6].Flag.Int64("size", -1, "size of the rom to lookup")
	cmd.Subcommands[6].Flag.String("out", "", "output dir")

	cmd.Subcommands[7] = &commander.Command{
		Run:       rs.progress,
		UsageLine: "progress",
		Short:     "Shows progress of the currently running command.",
		Long: `
Shows progress of the currently running command.`,
		Flag:   *flag.NewFlagSet("romba-progress", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[8] = &commander.Command{
		Run:       rs.shutdown,
		UsageLine: "shutdown",
		Short:     "Gracefully shuts down server.",
		Long: `
Gracefully shuts down server saving all the cached data.`,
		Flag:   *flag.NewFlagSet("romba-shutdown", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[9] = &commander.Command{
		Run:       rs.memstats,
		UsageLine: "memstats",
		Short:     "Prints memory stats.",
		Long: `
Print memory stats.`,
		Flag:   *flag.NewFlagSet("romba-memstats", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[10] = &commander.Command{
		Run:       rs.dbstats,
		UsageLine: "dbstats",
		Short:     "Prints db stats.",
		Long: `
Print db stats.`,
		Flag:   *flag.NewFlagSet("romba-dbstats", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[11] = &commander.Command{
		Run:       rs.cancel,
		UsageLine: "cancel",
		Short:     "Cancels current long-running job",
		Long: `
Cancels current long-running job.`,
		Flag:   *flag.NewFlagSet("romba-cancel", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[12] = &commander.Command{
		Run:       rs.startMerge,
		UsageLine: "merge",
		Short:     "Merges depot",
		Long: `
Merges specified depot into current depot.`,
		Flag:   *flag.NewFlagSet("romba-merge", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[12].Flag.Bool("only-needed", false, "only merge ROM files actually referenced by DAT files from the DAT index")
	cmd.Subcommands[12].Flag.String("resume", "", "resume a previously interrupted merge operation from the specified path")
	cmd.Subcommands[12].Flag.Int("workers", config.GlobalConfig.General.Workers,
		"how many workers to launch for the job")
	cmd.Subcommands[12].Flag.Bool("skip-initial-scan", false, "skip the initial scan of the files to determine amount of work")

	cmd.Subcommands[13] = &commander.Command{
		Run:       rs.printVersion,
		UsageLine: "version",
		Short:     "Prints version",
		Long: `
Prints version.`,
		Flag:   *flag.NewFlagSet("romba-version", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[14] = &commander.Command{
		Run:       rs.ediffdat,
		UsageLine: "ediffdat -old <dat dir> -new <dat dir> -out <output dir>",
		Short:     "Creates a DAT file with those entries that are in -new DAT.",
		Long: `
Creates a DAT file with those entries that are in -new DAT files and not
in -old DAT files. Ignores those entries in -old that are not in -new.`,
		Flag:   *flag.NewFlagSet("romba-ediffdat", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[14].Flag.String("out", "", "output dir")
	cmd.Subcommands[14].Flag.String("old", "", "old DAT file")
	cmd.Subcommands[14].Flag.String("new", "", "new DAT file")

	cmd.Subcommands[15] = &commander.Command{
		Run:       rs.datstats,
		UsageLine: "datstats",
		Short:     "Prints dat stats.",
		Long: `
Print dat stats.`,
		Flag:   *flag.NewFlagSet("romba-datstats", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[16] = &commander.Command{
		Run:       rs.export,
		UsageLine: "export",
		Short:     "Exports the hashes associations as a DAT file.",
		Long: `
Exports the hashes associations as a DAT file.`,
		Flag:   *flag.NewFlagSet("romba-export", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[16].Flag.String("out", "", "output DAT file")

	cmd.Subcommands[17] = &commander.Command{
		Run:       rs.imprt,
		UsageLine: "import",
		Short:     "Import the hashes associations as a DAT file.",
		Long: `
Imports the hashes associations as a DAT file.`,
		Flag:   *flag.NewFlagSet("romba-import", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[17].Flag.String("in", "", "input DAT file")

	cmd.Subcommands[18] = &commander.Command{
		Run:       rs.popBloom,
		UsageLine: "popbloom",
		Short:     "Populate the bloom filter.",
		Long: `
Populate the bloom filter.`,
		Flag:   *flag.NewFlagSet("romba-popbloom", flag.ContinueOnError),
		Stdout: writer,
		Stderr: writer,
	}

	cmd.Subcommands[18].Flag.Int("workers", config.GlobalConfig.General.Workers,
		"how many workers to launch for the job")

	cmd.Subcommands[18].Flag.Int("subworkers", config.GlobalConfig.General.Workers,
		"how many subworkers to launch for each worker")

	return cmd
}
