package expreduce

// This function assumes e and lhs have the same head and that the head is Flat.
func FlatReplace(e *Expression, lhs *Expression, rhs Ex, orderless bool, es *EvalState) Ex {
	looseLhs := NewExpression([]Ex{})
	looseLhs.Parts = append(looseLhs.Parts, lhs.Parts[0])
	if !orderless {
		looseLhs.Parts = append(looseLhs.Parts, NewExpression([]Ex{
			NewSymbol("System`Pattern"),
			NewSymbol("System`Expreduce`start"),
			NewExpression([]Ex{NewSymbol("System`BlankNullSequence")}),
		}))
	}
	looseLhs.Parts = append(looseLhs.Parts, lhs.Parts[1:]...)
	looseLhs.Parts = append(looseLhs.Parts, NewExpression([]Ex{
		NewSymbol("System`Pattern"),
		NewSymbol("System`Expreduce`end"),
		NewExpression([]Ex{NewSymbol("System`BlankNullSequence")}),
	}))
	pm := EmptyPD()
	matchq, newPd := IsMatchQ(e, looseLhs, pm, es)
	if matchq {
		var tmpEx Ex
		if orderless {
			tmpEx = ReplacePD(NewExpression([]Ex{
				e.Parts[0],
				rhs,
				NewSymbol("System`Expreduce`end"),
			}), es, newPd)
		} else {
			tmpEx = ReplacePD(NewExpression([]Ex{
				e.Parts[0],
				NewSymbol("System`Expreduce`start"),
				rhs,
				NewSymbol("System`Expreduce`end"),
			}), es, newPd)
		}
		return tmpEx
	}
	return e
}

func ReplacePDInternal(e Ex, pm *PDManager) (Ex, bool) {
	asSym, isSym := e.(*Symbol)
	if isSym {
		for k, def := range pm.patternDefined {
			if k == asSym.Name {
				// Shouldn't need the copy
				return def, true
			}
		}
	}
	thisDirty := false
	asExpr, isExpr := e.(*Expression)
	if isExpr {
		for i := range asExpr.Parts {
			possiblyNewExpr, dirty := ReplacePDInternal(asExpr.Parts[i], pm)
			if dirty {
				thisDirty = true
				// Mark the expression as dirty and needing eval.
				asExpr.evaledHash = 0
				asExpr.cachedHash = 0
			}
			asExpr.Parts[i] = possiblyNewExpr
		}
	}
	return e, thisDirty
}

func ReplacePD(this Ex, es *EvalState, pm *PDManager) Ex {
	if pm == nil {
		return this
	}
	containsAny := false
	for k := range pm.patternDefined {
		if ContainsSymbol(this, k) {
			containsAny = true
			break
		}
	}
	if !containsAny {
		return this
	}

	// Expressions are immutable. Any time we change an expression, we must
	// first copy it.
	res, _ := ReplacePDInternal(this.Copy(), pm)
	return res
}

// The goal of this function is to replace all matching expressions with the
// RHS upon successful matches. We will NOT substitute any named patterns in
// the RHS. We will merely make sure that the named patterns are added to pm.
// Final named pattern substitution will occur at the last possible time.
func ReplaceAll(this Ex, r *Expression, es *EvalState, pm *PDManager,
	stopAtHead string) Ex {
	asExpression, isExpression := this.(*Expression)

	if isExpression {
		_, isRestrictedHead := HeadAssertion(this, stopAtHead)
		if isRestrictedHead {
			return this
		} else {
			// Continue recursion
			es.Debugf("ReplaceAll(%v, %v, es, %v)", this, r, pm)
			return asExpression.ReplaceAll(r, stopAtHead, es)
		}
	}
	if res, matches := IsMatchQ(this, r.Parts[1], pm, es); res {
		return ReplacePD(r.Parts[2], es, matches)
	}
	return this
}

func tryCondWithMatches(rhs Ex, matches *PDManager, es *EvalState) (Ex, bool) {
	asCond, isCond := HeadAssertion(rhs, "System`Condition")
	if !isCond {
		if asWith, isWith := HeadAssertion(rhs, "System`With"); isWith {
			if len(asWith.Parts) == 3 {
				if _, hasCond := HeadAssertion(asWith.Parts[2], "System`Condition"); hasCond {
					appliedWith, ok := applyWithFn(asWith, es)
					if ok {
						asCond, isCond = HeadAssertion(appliedWith, "System`Condition")
					}
				}
			}
		}
		if asMod, isMod := HeadAssertion(rhs, "System`Module"); isMod {
			if len(asMod.Parts) == 3 {
				if _, hasCond := HeadAssertion(asMod.Parts[2], "System`Condition"); hasCond {
					appliedMod, ok := applyModuleFn(asMod, es)
					if ok {
						asCond, isCond = HeadAssertion(appliedMod, "System`Condition")
					}
				}
			}
		}
	}
	if isCond {
		condRes := asCond.Parts[2].Eval(es)
		condResSymbol, condResIsSymbol := condRes.(*Symbol)
		if condResIsSymbol {
			if condResSymbol.Name == "System`True" {
				return tryCondWithMatches(asCond.Parts[1], matches, es)
			}
		}
		return nil, false
	}
	return rhs, true
}

func Replace(this Ex, r *Expression, es *EvalState) (Ex, bool) {
	mi, cont := NewMatchIter(this, r.Parts[1], EmptyPD(), es)
	for cont {
		res, matches, done := mi.next()
		cont = !done
		if res {
			replacedRhs := ReplacePD(r.Parts[2], es, matches)
			toReturn, ok := tryCondWithMatches(replacedRhs, matches, es)
			if ok {
				return toReturn, true
			}
		}
	}
	return this, false
}
