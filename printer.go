// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package Printer

import (
	"os";
	"array";
	"tabwriter";
	"flag";
	"fmt";
	Scanner "scanner";
	AST "ast";
)

var (
	debug = flag.Bool("debug", false, nil, "print debugging information");
	
	// layout control
	tabwidth = flag.Int("tabwidth", 8, nil, "tab width");
	usetabs = flag.Bool("usetabs", true, nil, "align with tabs instead of blanks");
	newlines = flag.Bool("newlines", true, nil, "respect newlines in source");
	maxnewlines = flag.Int("maxnewlines", 3, nil, "max. number of consecutive newlines");

	// formatting control
	comments = flag.Bool("comments", true, nil, "print comments");
	optsemicolons = flag.Bool("optsemicolons", false, nil, "print optional semicolons");
)


// ----------------------------------------------------------------------------
// Printer

// Separators - printed in a delayed fashion, depending on context.
const (
	none = iota;
	blank;
	tab;
	comma;
	semicolon;
)


// Semantic states - control formatting.
const (
	normal = iota;
	opening_scope;  // controls indentation, scope level
	closing_scope;  // controls indentation, scope level
	inside_list;  // controls extra line breaks
)


type Printer struct {
	// output
	writer *tabwriter.Writer;
	
	// comments
	comments *array.Array;  // the list of all comments
	cindex int;  // the current comments index
	cpos int;  // the position of the next comment

	// current state
	lastpos int;  // pos after last string
	level int;  // scope level
	indentation int;  // indentation level (may be different from scope level)
	
	// formatting parameters
	separator int;  // pending separator
	newlines int;  // pending newlines
	
	// semantic state
	state int;  // current semantic state
	laststate int;  // state for last string
}


func (P *Printer) HasComment(pos int) bool {
	return comments.BVal() && P.cpos < pos;
}


func (P *Printer) NextComment() {
	P.cindex++;
	if P.comments != nil && P.cindex < P.comments.Len() {
		P.cpos = P.comments.At(P.cindex).(*AST.Comment).pos;
	} else {
		P.cpos = 1<<30;  // infinite
	}
}


func (P *Printer) Init(writer *tabwriter.Writer, comments *array.Array) {
	// writer
	padchar := byte(' ');
	if usetabs.BVal() {
		padchar = '\t';
	}
	P.writer = tabwriter.New(os.Stdout, int(tabwidth.IVal()), 1, padchar, true);

	// comments
	P.comments = comments;
	P.cindex = -1;
	P.NextComment();
	
	// formatting parameters & semantic state initialized correctly by default
}


// ----------------------------------------------------------------------------
// Printing support

func (P *Printer) Printf(format string, s ...) {
	n, err := fmt.fprintf(P.writer, format, s);
	if err != nil {
		panic("print error - exiting");
	}
}


func (P *Printer) Newline(n int) {
	if n > 0 {
		m := int(maxnewlines.IVal());
		if n > m {
			n = m;
		}
		for ; n > 0; n-- {
			P.Printf("\n");
		}
		for i := P.indentation; i > 0; i-- {
			P.Printf("\t");
		}
	}
}


func (P *Printer) String(pos int, s string) {
	// use estimate for pos if we don't have one
	if pos == 0 {
		pos = P.lastpos;
	}

	// --------------------------------
	// print pending separator, if any
	// - keep track of white space printed for better comment formatting
	// TODO print white space separators after potential comments and newlines
	// (currently, we may get trailing white space before a newline)
	trailing_char := 0;
	switch P.separator {
	case none:	// nothing to do
	case blank:
		P.Printf(" ");
		trailing_char = ' ';
	case tab:
		P.Printf("\t");
		trailing_char = '\t';
	case comma:
		P.Printf(",");
		if P.newlines == 0 {
			P.Printf(" ");
			trailing_char = ' ';
		}
	case semicolon:
		if P.level > 0 {	// no semicolons at level 0
			P.Printf(";");
			if P.newlines == 0 {
				P.Printf(" ");
				trailing_char = ' ';
			}
		}
	default:	panic("UNREACHABLE");
	}
	P.separator = none;

	// --------------------------------
	// interleave comments, if any
	nlcount := 0;
	for ; P.HasComment(pos); P.NextComment() {
		// we have a comment/newline that comes before the string
		comment := P.comments.At(P.cindex).(*AST.Comment);
		ctext := comment.text;
		
		if ctext == "\n" {
			// found a newline in src - count it
			nlcount++;

		} else {
			// classify comment (len(ctext) >= 2)
			//-style comment
			if nlcount > 0 || P.cpos == 0 {
				// only white space before comment on this line
				// or file starts with comment
				// - indent
				if !newlines.BVal() && P.cpos != 0 {
					nlcount = 1;
				}
				P.Newline(nlcount);
				nlcount = 0;

			} else {
				// black space before comment on this line
				if ctext[1] == '/' {
					//-style comment
					// - put in next cell unless a scope was just opened
					//   in which case we print 2 blanks (otherwise the
					//   entire scope gets indented like the next cell)
					if P.laststate == opening_scope {
						switch trailing_char {
						case ' ': P.Printf(" ");  // one space already printed
						case '\t': // do nothing
						default: P.Printf("  ");
						}
					} else {
						if trailing_char != '\t' {
							P.Printf("\t");
						}
					}
				} else {
					/*-style comment */
					// - print surrounded by blanks
					if trailing_char == 0 {
						P.Printf(" ");
					}
					ctext += " ";
				}
			}
			
			// print comment
			if debug.BVal() {
				P.Printf("[%d]", P.cpos);
			}
			P.Printf("%s", ctext);

			if ctext[1] == '/' {
				//-style comments must end in newline
				if P.newlines == 0 {  // don't add newlines if not needed
					P.newlines = 1;
				}
			}
		}
	}
	// At this point we may have nlcount > 0: In this case we found newlines
	// that were not followed by a comment. They are recognized (or not) when
	// printing newlines below.
	
	// --------------------------------
	// interpret state
	// (any pending separator or comment must be printed in previous state)
	switch P.state {
	case normal:
	case opening_scope:
	case closing_scope:
		P.indentation--;
	case inside_list:
	default:
		panic("UNREACHABLE");
	}

	// --------------------------------
	// print pending newlines
	if newlines.BVal() && (P.newlines > 0 || P.state == inside_list) && nlcount > P.newlines {
		// Respect additional newlines in the source, but only if we
		// enabled this feature (newlines.BVal()) and we are expecting
		// newlines (P.newlines > 0 || P.state == inside_list).
		// Otherwise - because we don't have all token positions - we
		// get funny formatting.
		P.newlines = nlcount;
	}
	nlcount = 0;
	P.Newline(P.newlines);
	P.newlines = 0;

	// --------------------------------
	// print string
	if debug.BVal() {
		P.Printf("[%d]", pos);
	}
	P.Printf("%s", s);

	// --------------------------------
	// interpret state
	switch P.state {
	case normal:
	case opening_scope:
		P.level++;
		P.indentation++;
	case closing_scope:
		P.level--;
	case inside_list:
	default:
		panic("UNREACHABLE");
	}
	P.laststate = P.state;
	P.state = none;

	// --------------------------------
	// done
	P.lastpos = pos + len(s);  // rough estimate
}


func (P *Printer) Separator(separator int) {
	P.separator = separator;
	P.String(0, "");
}


func (P *Printer) Token(pos int, tok int) {
	P.String(pos, Scanner.TokenString(tok));
}


func (P *Printer) Error(pos int, tok int, msg string) {
	P.String(0, "<");
	P.Token(pos, tok);
	P.String(0, " " + msg + ">");
}


// ----------------------------------------------------------------------------
// Types

func (P *Printer) Type(t *AST.Type)
func (P *Printer) Expr(x *AST.Expr)

func (P *Printer) Parameters(pos int, list *array.Array) {
	P.String(pos, "(");
	if list != nil {
		var prev int;
		for i, n := 0, list.Len(); i < n; i++ {
			x := list.At(i).(*AST.Expr);
			if i > 0 {
				if prev == x.tok || prev == Scanner.TYPE {
					P.Separator(comma);
				} else {
					P.Separator(blank);
				}
			}
			P.Expr(x);
			prev = x.tok;
		}
	}
	P.String(0, ")");
}


func (P *Printer) Fields(list *array.Array, end int) {
	P.state = opening_scope;
	P.String(0, "{");

	if list != nil {
		P.newlines = 1;
		var prev int;
		for i, n := 0, list.Len(); i < n; i++ {
			x := list.At(i).(*AST.Expr);
			if i > 0 {
				if prev == Scanner.TYPE && x.tok != Scanner.STRING || prev == Scanner.STRING {
					P.separator = semicolon;
					P.newlines = 1;
				} else if prev == x.tok {
					P.separator = comma;
				} else {
					P.separator = tab;
				}
			}
			P.Expr(x);
			prev = x.tok;
		}
		P.newlines = 1;
	}

	P.state = closing_scope;
	P.String(end, "}");
}


func (P *Printer) Type(t *AST.Type) {
	switch t.tok {
	case Scanner.IDENT:
		P.Expr(t.expr);

	case Scanner.LBRACK:
		P.String(t.pos, "[");
		if t.expr != nil {
			P.Expr(t.expr);
		}
		P.String(0, "]");
		P.Type(t.elt);

	case Scanner.STRUCT, Scanner.INTERFACE:
		P.Token(t.pos, t.tok);
		if t.list != nil {
			P.separator = blank;
			P.Fields(t.list, t.end);
		}

	case Scanner.MAP:
		P.String(t.pos, "map [");
		P.Type(t.key);
		P.String(0, "]");
		P.Type(t.elt);

	case Scanner.CHAN:
		var m string;
		switch t.mode {
		case AST.FULL: m = "chan ";
		case AST.RECV: m = "<-chan ";
		case AST.SEND: m = "chan <- ";
		}
		P.String(t.pos, m);
		P.Type(t.elt);

	case Scanner.MUL:
		P.String(t.pos, "*");
		P.Type(t.elt);

	case Scanner.LPAREN:
		P.Parameters(t.pos, t.list);
		if t.elt != nil {
			P.separator = blank;
			list := t.elt.list;
			if list.Len() > 1 {
				P.Parameters(0, list);
			} else {
				// single, anonymous result type
				P.Expr(list.At(0).(*AST.Expr));
			}
		}

	case Scanner.ELLIPSIS:
		P.String(t.pos, "...");

	default:
		P.Error(t.pos, t.tok, "type");
	}
}


// ----------------------------------------------------------------------------
// Expressions

func (P *Printer) Block(pos int, list *array.Array, end int, indent bool);

func (P *Printer) Expr1(x *AST.Expr, prec1 int) {
	if x == nil {
		return;  // empty expression list
	}

	switch x.tok {
	case Scanner.TYPE:
		// type expr
		P.Type(x.t);

	case Scanner.IDENT, Scanner.INT, Scanner.STRING, Scanner.FLOAT:
		// literal
		P.String(x.pos, x.s);

	case Scanner.FUNC:
		// function literal
		P.String(x.pos, "func");
		P.Type(x.t);
		P.Block(0, x.block, x.end, true);
		P.newlines = 0;

	case Scanner.COMMA:
		// list
		// (don't use binary expression printing because of different spacing)
		P.Expr(x.x);
		P.String(x.pos, ",");
		P.separator = blank;
		P.state = inside_list;
		P.Expr(x.y);

	case Scanner.PERIOD:
		// selector or type guard
		P.Expr1(x.x, Scanner.HighestPrec);
		P.String(x.pos, ".");
		if x.y != nil {
			P.Expr1(x.y, Scanner.HighestPrec);
		} else {
			P.String(0, "(");
			P.Type(x.t);
			P.String(0, ")");
		}
		
	case Scanner.LBRACK:
		// index
		P.Expr1(x.x, Scanner.HighestPrec);
		P.String(x.pos, "[");
		P.Expr1(x.y, 0);
		P.String(0, "]");

	case Scanner.LPAREN:
		// call
		P.Expr1(x.x, Scanner.HighestPrec);
		P.String(x.pos, "(");
		P.Expr(x.y);
		P.String(0, ")");

	case Scanner.LBRACE:
		// composite
		P.Type(x.t);
		P.String(x.pos, "{");
		P.Expr(x.y);
		P.String(0, "}");
		
	default:
		// unary and binary expressions including ":" for pairs
		prec := Scanner.UnaryPrec;
		if x.x != nil {
			prec = Scanner.Precedence(x.tok);
		}
		if prec < prec1 {
			P.String(0, "(");
		}
		if x.x == nil {
			// unary expression
			P.Token(x.pos, x.tok);
		} else {
			// binary expression
			P.Expr1(x.x, prec);
			P.separator = blank;
			P.Token(x.pos, x.tok);
			P.separator = blank;
		}
		P.Expr1(x.y, prec);
		if prec < prec1 {
			P.String(0, ")");
		}
	}
}


func (P *Printer) Expr(x *AST.Expr) {
	P.Expr1(x, Scanner.LowestPrec);
}


// ----------------------------------------------------------------------------
// Statements

func (P *Printer) Stat(s *AST.Stat)

func (P *Printer) StatementList(list *array.Array) {
	if list != nil {
		P.newlines = 1;
		for i, n := 0, list.Len(); i < n; i++ {
			P.Stat(list.At(i).(*AST.Stat));
			P.newlines = 1;
		}
	}
}


func (P *Printer) Block(pos int, list *array.Array, end int, indent bool) {
	P.state = opening_scope;
	P.String(pos, "{");
	if !indent {
		P.indentation--;
	}
	P.StatementList(list);
	if !indent {
		P.indentation++;
	}
	if !optsemicolons.BVal() {
		P.separator = none;
	}
	P.state = closing_scope;
	P.String(end, "}");
}


func (P *Printer) ControlClause(s *AST.Stat) {
	has_post := s.tok == Scanner.FOR && s.post != nil;  // post also used by "if"

	P.separator = blank;
	if s.init == nil && !has_post {
		// no semicolons required
		if s.expr != nil {
			P.Expr(s.expr);
		}
	} else {
		// all semicolons required
		// (they are not separators, print them explicitly)
		if s.init != nil {
			P.Stat(s.init);
			P.separator = none;
		}
		P.Printf(";");
		P.separator = blank;
		if s.expr != nil {
			P.Expr(s.expr);
			P.separator = none;
		}
		if s.tok == Scanner.FOR {
			P.Printf(";");
			P.separator = blank;
			if has_post {
				P.Stat(s.post);
			}
		}
	}
	P.separator = blank;
}


func (P *Printer) Declaration(d *AST.Decl, parenthesized bool);

func (P *Printer) Stat(s *AST.Stat) {
	switch s.tok {
	case Scanner.EXPRSTAT:
		// expression statement
		P.Expr(s.expr);
		P.separator = semicolon;

	case Scanner.COLON:
		// label declaration
		P.indentation--;
		P.Expr(s.expr);
		P.Token(s.pos, s.tok);
		P.indentation++;
		P.separator = none;
		
	case Scanner.CONST, Scanner.TYPE, Scanner.VAR:
		// declaration
		P.Declaration(s.decl, false);

	case Scanner.INC, Scanner.DEC:
		P.Expr(s.expr);
		P.Token(s.pos, s.tok);
		P.separator = semicolon;

	case Scanner.LBRACE:
		// block
		P.Block(s.pos, s.block, s.end, true);

	case Scanner.IF:
		P.String(s.pos, "if");
		P.ControlClause(s);
		P.Block(0, s.block, s.end, true);
		if s.post != nil {
			P.separator = blank;
			P.String(0, "else");
			P.separator = blank;
			P.Stat(s.post);
		}

	case Scanner.FOR:
		P.String(s.pos, "for");
		P.ControlClause(s);
		P.Block(0, s.block, s.end, true);

	case Scanner.SWITCH, Scanner.SELECT:
		P.Token(s.pos, s.tok);
		P.ControlClause(s);
		P.Block(0, s.block, s.end, false);

	case Scanner.CASE, Scanner.DEFAULT:
		P.Token(s.pos, s.tok);
		if s.expr != nil {
			P.separator = blank;
			P.Expr(s.expr);
		}
		P.String(0, ":");
		P.indentation++;
		P.StatementList(s.block);
		P.indentation--;
		P.newlines = 1;

	case Scanner.GO, Scanner.RETURN, Scanner.FALLTHROUGH, Scanner.BREAK, Scanner.CONTINUE, Scanner.GOTO:
		P.Token(s.pos, s.tok);
		if s.expr != nil {
			P.separator = blank;
			P.Expr(s.expr);
		}
		P.separator = semicolon;

	default:
		P.Error(s.pos, s.tok, "stat");
	}
}


// ----------------------------------------------------------------------------
// Declarations

// TODO This code is unreadable! Clean up AST and rewrite this.

func (P *Printer) Declaration(d *AST.Decl, parenthesized bool) {
	if !parenthesized {
		if d.exported {
			P.String(d.pos, "export");
			P.separator = blank;
		}
		P.Token(d.pos, d.tok);
		P.separator = blank;
	}

	if d.tok != Scanner.FUNC && d.list != nil {
		P.state = opening_scope;
		P.String(0, "(");
		if d.list.Len() > 0 {
			P.newlines = 1;
			for i := 0; i < d.list.Len(); i++ {
				P.Declaration(d.list.At(i).(*AST.Decl), true);
				P.separator = semicolon;
				P.newlines = 1;
			}
		}
		P.state = closing_scope;
		P.String(d.end, ")");

	} else {
		if d.tok == Scanner.FUNC && d.typ.key != nil {
			P.Parameters(0, d.typ.key.list);
			P.separator = blank;
		}

		P.Expr(d.ident);
		
		if d.typ != nil {
			if d.tok != Scanner.FUNC {
				// TODO would like to change this to a tab separator
				// but currently this causes trouble when the type is
				// a struct/interface (fields are indented wrongly)
				P.separator = blank;
			}
			P.Type(d.typ);
			P.separator = tab;
		}

		if d.val != nil {
			if d.tok != Scanner.IMPORT {
				P.separator = tab;
				P.String(0, "=");
				P.separator = blank;
			}
			P.Expr(d.val);
		}

		if d.list != nil {
			if d.tok != Scanner.FUNC {
				panic("must be a func declaration");
			}
			P.separator = blank;
			P.Block(0, d.list, d.end, true);
		}
		
		if d.tok != Scanner.TYPE {
			P.separator = semicolon;
		}
	}
	
	P.newlines = 2;
}


// ----------------------------------------------------------------------------
// Program

func (P *Printer) Program(p *AST.Program) {
	P.String(p.pos, "package");
	P.separator = blank;
	P.Expr(p.ident);
	P.newlines = 1;
	for i := 0; i < p.decls.Len(); i++ {
		P.Declaration(p.decls.At(i), false);
	}
	P.newlines = 1;
}


// ----------------------------------------------------------------------------
// External interface

export func Print(prog *AST.Program) {
	// setup
	padchar := byte(' ');
	if usetabs.BVal() {
		padchar = '\t';
	}
	writer := tabwriter.New(os.Stdout, int(tabwidth.IVal()), 1, padchar, true);
	var P Printer;
	P.Init(writer, prog.comments);

	P.Program(prog);
	
	// flush
	P.String(0, "");
	err := P.writer.Flush();
	if err != nil {
		panic("print error - exiting");
	}
}
