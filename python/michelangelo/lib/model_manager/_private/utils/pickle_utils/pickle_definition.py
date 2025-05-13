from __future__ import annotations
import pickletools


def find_pickle_definitions(pickled_file: str) -> list[str]:
    """
    Find the Python definitions needed to unpickle the pickled file.

    Args:
        pickled_file: the path of the pickled file

    Returns:
        A list of Python import paths, e.g., "module.submodule.Class"
    """
    stack = []
    markstack = []
    memo = []
    result = set()
    mo = pickletools.markobject

    with open(pickled_file, "rb") as f:
        for op, arg, pos in pickletools.genops(f):
            before, after = op.stack_before, op.stack_after
            numtopop = len(before)

            if op.name == "GLOBAL":  # GLOBAL is only used before Protocol 4
                result.add(".".join(arg.split()))
            elif op.name == "STACK_GLOBAL":
                result.add(".".join(stack[-2:]))

            elif mo in before or (op.name == "POP" and stack and stack[-1] is mo):
                markstack.pop()
                while stack[-1] is not mo:
                    stack.pop()
                stack.pop()
                try:
                    numtopop = before.index(mo)
                except ValueError:
                    numtopop = 0
            elif op.name in {"PUT", "BINPUT", "LONG_BINPUT", "MEMOIZE"}:
                if op.name == "MEMOIZE":
                    memo.append(stack[-1])
                else:  # PUT, BINPUT, LONG_BINPUT are only used before Protocol 4
                    while len(memo) <= arg:
                        memo.append(None)
                    memo[arg] = stack[-1]
                numtopop, after = 0, []
            elif op.name in {"GET", "BINGET", "LONG_BINGET"}:
                arg = memo[arg]

            if numtopop:
                del stack[-numtopop:]
            if mo in after:
                markstack.append(pos)

            if len(after) == 1 and op.arg is not None:
                stack.append(arg)
            else:
                stack.extend(after)

    return list(result)
