// Copyright 2020 The Ebiten Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package shader

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/hajimehoshi/ebiten/internal/shaderir"
)

func (cs *compileState) parseStmt(block *block, stmt ast.Stmt, inParams []variable) bool {
	switch stmt := stmt.(type) {
	case *ast.AssignStmt:
		switch stmt.Tok {
		case token.DEFINE:
			if len(stmt.Lhs) != len(stmt.Rhs) && len(stmt.Rhs) != 1 {
				cs.addError(stmt.Pos(), fmt.Sprintf("single-value context and multiple-value context cannot be mixed"))
				return false
			}

			if !cs.assign(block, stmt.Pos(), stmt.Lhs, stmt.Rhs, true) {
				return false
			}
		case token.ASSIGN:
			// TODO: What about the statement `a,b = b,a?`
			if len(stmt.Lhs) != len(stmt.Rhs) && len(stmt.Rhs) != 1 {
				cs.addError(stmt.Pos(), fmt.Sprintf("single-value context and multiple-value context cannot be mixed"))
				return false
			}
			if !cs.assign(block, stmt.Pos(), stmt.Lhs, stmt.Rhs, false) {
				return false
			}
		case token.ADD_ASSIGN, token.SUB_ASSIGN, token.MUL_ASSIGN, token.QUO_ASSIGN, token.REM_ASSIGN:
			var op shaderir.Op
			switch stmt.Tok {
			case token.ADD_ASSIGN:
				op = shaderir.Add
			case token.SUB_ASSIGN:
				op = shaderir.Sub
			case token.MUL_ASSIGN:
				op = shaderir.Mul
			case token.QUO_ASSIGN:
				op = shaderir.Div
			case token.REM_ASSIGN:
				op = shaderir.ModOp
			}
			rhs, _, stmts, ok := cs.parseExpr(block, stmt.Rhs[0])
			if !ok {
				return false
			}
			block.ir.Stmts = append(block.ir.Stmts, stmts...)
			lhs, _, stmts, ok := cs.parseExpr(block, stmt.Lhs[0])
			if !ok {
				return false
			}
			block.ir.Stmts = append(block.ir.Stmts, stmts...)
			block.ir.Stmts = append(block.ir.Stmts, shaderir.Stmt{
				Type: shaderir.Assign,
				Exprs: []shaderir.Expr{
					lhs[0],
					{
						Type: shaderir.Binary,
						Op:   op,
						Exprs: []shaderir.Expr{
							lhs[0],
							rhs[0],
						},
					},
				},
			})
		default:
			cs.addError(stmt.Pos(), fmt.Sprintf("unexpected token: %s", stmt.Tok))
		}
	case *ast.BlockStmt:
		b, ok := cs.parseBlock(block, stmt, nil, nil)
		if !ok {
			return false
		}
		block.ir.Stmts = append(block.ir.Stmts, shaderir.Stmt{
			Type: shaderir.BlockStmt,
			Blocks: []shaderir.Block{
				b.ir,
			},
		})
	case *ast.DeclStmt:
		if !cs.parseDecl(block, stmt.Decl) {
			return false
		}
	case *ast.ReturnStmt:
		for i, r := range stmt.Results {
			exprs, _, stmts, ok := cs.parseExpr(block, r)
			if !ok {
				return false
			}
			block.ir.Stmts = append(block.ir.Stmts, stmts...)
			if len(exprs) == 0 {
				continue
			}
			if len(exprs) > 1 {
				cs.addError(r.Pos(), "multiple-context with return is not implemented yet")
				continue
			}
			block.ir.Stmts = append(block.ir.Stmts, shaderir.Stmt{
				Type: shaderir.Assign,
				Exprs: []shaderir.Expr{
					{
						Type:  shaderir.LocalVariable,
						Index: len(inParams) + i,
					},
					exprs[0],
				},
			})
		}
		block.ir.Stmts = append(block.ir.Stmts, shaderir.Stmt{
			Type: shaderir.Return,
		})
	case *ast.ExprStmt:
		exprs, _, stmts, ok := cs.parseExpr(block, stmt.X)
		if !ok {
			return false
		}
		block.ir.Stmts = append(block.ir.Stmts, stmts...)
		for _, expr := range exprs {
			if expr.Type != shaderir.Call {
				continue
			}
			block.ir.Stmts = append(block.ir.Stmts, shaderir.Stmt{
				Type:  shaderir.ExprStmt,
				Exprs: []shaderir.Expr{expr},
			})
		}
	default:
		cs.addError(stmt.Pos(), fmt.Sprintf("unexpected statement: %#v", stmt))
		return false
	}
	return true
}

func (cs *compileState) assign(block *block, pos token.Pos, lhs, rhs []ast.Expr, define bool) bool {
	var rhsExprs []shaderir.Expr
	var rhsTypes []shaderir.Type

	for i, e := range lhs {
		if len(lhs) == len(rhs) {
			// Prase RHS first for the order of the statements.
			r, origts, stmts, ok := cs.parseExpr(block, rhs[i])
			if !ok {
				return false
			}
			block.ir.Stmts = append(block.ir.Stmts, stmts...)

			if define {
				v := variable{
					name: e.(*ast.Ident).Name,
				}
				ts, ok := cs.functionReturnTypes(block, rhs[i])
				if !ok {
					ts = origts
				}
				if len(ts) > 1 {
					cs.addError(pos, fmt.Sprintf("single-value context and multiple-value context cannot be mixed"))
					return false
				}
				if len(ts) == 1 {
					v.typ = ts[0]
				}
				block.vars = append(block.vars, v)
			}

			if len(r) > 1 {
				cs.addError(pos, fmt.Sprintf("single-value context and multiple-value context cannot be mixed"))
				return false
			}

			l, _, stmts, ok := cs.parseExpr(block, lhs[i])
			if !ok {
				return false
			}
			block.ir.Stmts = append(block.ir.Stmts, stmts...)

			if r[0].Type == shaderir.NumberExpr {
				switch block.vars[l[0].Index].typ.Main {
				case shaderir.Int:
					r[0].ConstType = shaderir.ConstTypeInt
				case shaderir.Float:
					r[0].ConstType = shaderir.ConstTypeFloat
				}
			}

			block.ir.Stmts = append(block.ir.Stmts, shaderir.Stmt{
				Type:  shaderir.Assign,
				Exprs: []shaderir.Expr{l[0], r[0]},
			})
		} else {
			if i == 0 {
				var stmts []shaderir.Stmt
				var ok bool
				rhsExprs, rhsTypes, stmts, ok = cs.parseExpr(block, rhs[0])
				if !ok {
					return false
				}
				if len(rhsExprs) != len(lhs) {
					cs.addError(pos, fmt.Sprintf("single-value context and multiple-value context cannot be mixed"))
				}
				block.ir.Stmts = append(block.ir.Stmts, stmts...)
			}

			if define {
				v := variable{
					name: e.(*ast.Ident).Name,
				}
				v.typ = rhsTypes[i]
				block.vars = append(block.vars, v)
			}

			l, _, stmts, ok := cs.parseExpr(block, lhs[i])
			if !ok {
				return false
			}
			block.ir.Stmts = append(block.ir.Stmts, stmts...)

			block.ir.Stmts = append(block.ir.Stmts, shaderir.Stmt{
				Type:  shaderir.Assign,
				Exprs: []shaderir.Expr{l[0], rhsExprs[i]},
			})
		}
	}
	return true
}
