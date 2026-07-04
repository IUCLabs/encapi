package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/IUCLabs/encapi/pkg/client"
	"github.com/IUCLabs/encapi/pkg/models"
)

func nodeCmd(ctx context.Context, c *client.Client, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: encapi-cli node <list|get|set|delete> ...")
		return 2
	}
	switch args[0] {
	case "list":
		nodes, err := c.ListNodes(ctx)
		if err != nil {
			return fail(stderr, err)
		}
		printYAML(stdout, nodes)
		return 0
	case "get":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: encapi-cli node get <certname>")
			return 2
		}
		n, err := c.GetNode(ctx, args[1])
		if err != nil {
			return fail(stderr, err)
		}
		printYAML(stdout, n)
		return 0
	case "set":
		return nodeSet(ctx, c, args[1:], stdout, stderr)
	case "delete":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: encapi-cli node delete <certname>")
			return 2
		}
		if err := c.DeleteNode(ctx, args[1]); err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintf(stdout, "deleted node %s\n", args[1])
		return 0
	default:
		fmt.Fprintf(stderr, "unknown node subcommand %q\n", args[0])
		return 2
	}
}

func nodeSet(ctx context.Context, c *client.Client, args []string, stdout, stderr io.Writer) int {
	name, rest, ok := leadingName(args)
	if !ok {
		fmt.Fprintln(stderr, "usage: encapi-cli node set <certname> --role <r> --env <e> [--param k=v ...]")
		return 2
	}
	fs := flag.NewFlagSet("node set", flag.ContinueOnError)
	fs.SetOutput(stderr)
	role := fs.String("role", "", "role (class) to assign")
	env := fs.String("env", "", "environment/status")
	var params stringsFlag
	fs.Var(&params, "param", "param key=value (repeatable)")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if *role == "" || *env == "" {
		fmt.Fprintln(stderr, "usage: encapi-cli node set <certname> --role <r> --env <e> [--param k=v ...]")
		return 2
	}
	p, err := parseParams(params)
	if err != nil {
		return fail(stderr, err)
	}
	n, err := c.PutNode(ctx, &models.Node{Certname: name, Role: *role, Environment: *env, Params: p})
	if err != nil {
		return fail(stderr, err)
	}
	printYAML(stdout, n)
	return 0
}

func roleCmd(ctx context.Context, c *client.Client, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: encapi-cli role <list|get|set|delete> ...")
		return 2
	}
	switch args[0] {
	case "list":
		roles, err := c.ListRoles(ctx)
		if err != nil {
			return fail(stderr, err)
		}
		printYAML(stdout, roles)
		return 0
	case "get":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: encapi-cli role get <name>")
			return 2
		}
		r, err := c.GetRole(ctx, args[1])
		if err != nil {
			return fail(stderr, err)
		}
		printYAML(stdout, r)
		return 0
	case "set":
		return roleSet(ctx, c, args[1:], stdout, stderr)
	case "delete":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: encapi-cli role delete <name>")
			return 2
		}
		if err := c.DeleteRole(ctx, args[1]); err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintf(stdout, "deleted role %s\n", args[1])
		return 0
	default:
		fmt.Fprintf(stderr, "unknown role subcommand %q\n", args[0])
		return 2
	}
}

func roleSet(ctx context.Context, c *client.Client, args []string, stdout, stderr io.Writer) int {
	name, rest, ok := leadingName(args)
	if !ok {
		fmt.Fprintln(stderr, "usage: encapi-cli role set <name> [--desc <d>] [--param k=v ...]")
		return 2
	}
	fs := flag.NewFlagSet("role set", flag.ContinueOnError)
	fs.SetOutput(stderr)
	desc := fs.String("desc", "", "description")
	var params stringsFlag
	fs.Var(&params, "param", "default param key=value (repeatable)")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	p, err := parseParams(params)
	if err != nil {
		return fail(stderr, err)
	}
	r, err := c.PutRole(ctx, &models.Role{Name: name, Description: *desc, DefaultParams: p})
	if err != nil {
		return fail(stderr, err)
	}
	printYAML(stdout, r)
	return 0
}

func statusCmd(ctx context.Context, c *client.Client, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: encapi-cli status <list|get|set|delete> ...")
		return 2
	}
	switch args[0] {
	case "list":
		statuses, err := c.ListStatuses(ctx)
		if err != nil {
			return fail(stderr, err)
		}
		printYAML(stdout, statuses)
		return 0
	case "get":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: encapi-cli status get <name>")
			return 2
		}
		s, err := c.GetStatus(ctx, args[1])
		if err != nil {
			return fail(stderr, err)
		}
		printYAML(stdout, s)
		return 0
	case "set":
		return statusSet(ctx, c, args[1:], stdout, stderr)
	case "delete":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: encapi-cli status delete <name>")
			return 2
		}
		if err := c.DeleteStatus(ctx, args[1]); err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintf(stdout, "deleted status %s\n", args[1])
		return 0
	default:
		fmt.Fprintf(stderr, "unknown status subcommand %q\n", args[0])
		return 2
	}
}

func statusSet(ctx context.Context, c *client.Client, args []string, stdout, stderr io.Writer) int {
	name, rest, ok := leadingName(args)
	if !ok {
		fmt.Fprintln(stderr, "usage: encapi-cli status set <name> [--desc <d>]")
		return 2
	}
	fs := flag.NewFlagSet("status set", flag.ContinueOnError)
	fs.SetOutput(stderr)
	desc := fs.String("desc", "", "description")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	s, err := c.PutStatus(ctx, &models.Status{Name: name, Description: *desc})
	if err != nil {
		return fail(stderr, err)
	}
	printYAML(stdout, s)
	return 0
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, err)
	return 1
}
