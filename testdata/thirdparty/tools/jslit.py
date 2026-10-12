"""Tiny, non-executing parser for JavaScript/TypeScript/PHP literal
expressions found in test files (strings with + concatenation, numbers,
true/false/null, arrays, objects, identifiers and simple calls).

Never evaluates anything: calls and identifiers are returned as tagged
tuples so the caller can decide what to accept.
"""


def blank_comments(src, php=False):
    """Replace // ... and /* ... */ comments (and PHP # comments when php=True,
    except #[...] attributes) by spaces, keeping offsets and newlines."""
    out = list(src)
    i, n = 0, len(src)
    while i < n:
        c = src[i]
        if c in "'\"`":
            q = c
            i += 1
            while i < n and src[i] != q:
                if src[i] == "\\":
                    i += 1
                i += 1
            i += 1
            continue
        if src.startswith("//", i) or (php and c == "#" and not src.startswith("#[", i)):
            j = src.find("\n", i)
            j = n if j < 0 else j
            for k in range(i, j):
                out[k] = " "
            i = j
            continue
        if src.startswith("/*", i):
            j = src.find("*/", i + 2)
            j = n if j < 0 else j + 2
            for k in range(i, j):
                if out[k] != "\n":
                    out[k] = " "
            i = j
            continue
        i += 1
    return "".join(out)


class ParseError(Exception):
    pass


class Parser:
    def __init__(self, src, pos=0, php=False):
        self.s = src
        self.i = pos
        self.php = php

    def ws(self):
        while self.i < len(self.s) and self.s[self.i] in " \t\r\n":
            self.i += 1

    def peek(self, k=1):
        self.ws()
        return self.s[self.i:self.i + k]

    def eat(self, tok):
        self.ws()
        if not self.s.startswith(tok, self.i):
            raise ParseError("expected %r at %d: %r" % (tok, self.i, self.s[self.i:self.i + 40]))
        self.i += len(tok)

    def string(self):
        self.ws()
        q = self.s[self.i]
        self.i += 1
        buf = []
        while True:
            if self.i >= len(self.s):
                raise ParseError("unterminated string")
            c = self.s[self.i]
            if c == q:
                self.i += 1
                break
            if c == "\\":
                nx = self.s[self.i + 1]
                if q == "'" and self.php:
                    # PHP single quotes: only \\ and \' are escapes
                    if nx in "\\'":
                        buf.append(nx)
                    else:
                        buf.append("\\" + nx)
                else:
                    buf.append({"n": "\n", "r": "\r", "t": "\t", "\\": "\\",
                                "'": "'", '"': '"', "`": "`", "$": "$"}.get(nx, "\\" + nx))
                self.i += 2
                continue
            if q == "`" and c == "$" and self.s.startswith("${", self.i):
                raise ParseError("template interpolation")
            if q == '"' and self.php and c == "$":
                raise ParseError("php interpolation")
            buf.append(c)
            self.i += 1
        return "".join(buf)

    def expr(self):
        v = self.primary()
        while True:
            self.ws()
            if self.s.startswith("+", self.i) and not self.s.startswith("++", self.i):
                self.i += 1
                w = self.primary()
                if isinstance(v, str) and isinstance(w, str):
                    v = v + w
                else:
                    v = ("binop", "+", v, w)
            elif self.php and self.s.startswith(".", self.i) and not self.s.startswith("...", self.i):
                self.i += 1
                w = self.primary()
                if isinstance(v, str) and isinstance(w, str):
                    v = v + w
                else:
                    v = ("binop", ".", v, w)
            else:
                return v

    def ident(self):
        self.ws()
        j = self.i
        while j < len(self.s) and (self.s[j].isalnum() or self.s[j] in "_$\\"):
            j += 1
        if j == self.i:
            raise ParseError("expected identifier at %d: %r" % (self.i, self.s[self.i:self.i + 40]))
        name = self.s[self.i:j]
        self.i = j
        return name

    def primary(self):
        c = self.peek()
        if c in "'\"`":
            return self.string()
        if c == "[":
            return self.array()
        if c == "{":
            return self.obj()
        if c == "(":
            self.eat("(")
            v = self.expr()
            self.eat(")")
            return v
        if c == "-" or c.isdigit():
            j = self.i
            if self.s[j] == "-":
                j += 1
            while j < len(self.s) and (self.s[j].isdigit() or self.s[j] in "._"):
                j += 1
            txt = self.s[self.i:j].replace("_", "")
            self.i = j
            return float(txt) if "." in txt else int(txt)
        if self.s.startswith("new ", self.i):
            self.i += 4
            name = self.dotted()
            args = self.args() if self.peek() == "(" else []
            return self.postfix(("new", name, args))
        name = self.dotted()
        if name in ("true", "false"):
            return name == "true"
        if name in ("null", "undefined"):
            return None
        if self.php and name == "array" and self.peek() == "(":
            return self.array(open_="(", close=")")
        if self.peek() == "(":
            return self.postfix(("call", name, self.args()))
        return self.postfix(("ident", name))

    def postfix(self, v):
        while True:
            c = self.peek()
            if c == "." and not self.s.startswith("...", self.i):
                self.i += 1
                name = self.ident()
                if self.peek() == "(":
                    v = ("method", v, name, self.args())
                else:
                    v = ("attr", v, name)
            elif self.php and self.peek(2) == "->":
                self.i += 2
                name = self.ident()
                if self.peek() == "(":
                    v = ("method", v, name, self.args())
                else:
                    v = ("attr", v, name)
            else:
                return v

    def dotted(self):
        name = self.ident()
        while self.peek() == "." and not self.s.startswith("..", self.i) and \
                self.i + 1 < len(self.s) and (self.s[self.i + 1].isalpha() or self.s[self.i + 1] == "_") and not self.php:
            # only plain dotted names like RRule.MONTHLY or Frequency.DAILY
            save = self.i
            self.i += 1
            nxt = self.ident()
            if self.peek() == "(":
                # method call on dotted name: leave for postfix
                self.i = save
                break
            name = name + "." + nxt
        if self.php:
            while self.peek(2) == "::":
                self.i += 2
                name = name + "::" + self.ident()
        return name

    def _named_arg(self):
        self.ws()
        j = self.i
        while j < len(self.s) and (self.s[j].isalnum() or self.s[j] == "_"):
            j += 1
        k = j
        while k < len(self.s) and self.s[k] in " \t":
            k += 1
        if j > self.i and k < len(self.s) and self.s[k] == ":" and self.s[k + 1:k + 2] != ":":
            name = self.s[self.i:j]
            self.i = k + 1
            return ("named", name, self.expr())
        return None

    def args(self):
        self.eat("(")
        out = []
        while True:
            if self.peek() == ")":
                self.eat(")")
                return out
            named = self._named_arg() if self.php else None
            out.append(named if named is not None else self.expr())
            if self.peek() == ",":
                self.eat(",")
            elif self.peek() != ")":
                raise ParseError("expected , or ) at %d: %r" % (self.i, self.s[self.i:self.i + 40]))

    def array(self, open_="[", close="]"):
        self.eat(open_)
        items = []
        keyed = []
        while True:
            if self.peek() == close:
                self.eat(close)
                return ("assoc", keyed) if keyed and not items else items if not keyed else ("mixed", items, keyed)
            v = self.expr()
            if self.php and self.peek(2) == "=>":
                self.eat("=>")
                keyed.append((v, self.expr()))
            else:
                items.append(v)
            if self.peek() == ",":
                self.eat(",")
            elif self.peek() != close:
                raise ParseError("expected , or %s at %d: %r" % (close, self.i, self.s[self.i:self.i + 40]))

    def obj(self):
        self.eat("{")
        d = {}
        while True:
            if self.peek() == "}":
                self.eat("}")
                return d
            c = self.peek()
            if c in "'\"":
                k = self.string()
            else:
                k = self.ident()
            if self.peek() == ":":
                self.eat(":")
                d[k] = self.expr()
            else:
                d[k] = ("ident", k)  # shorthand property
            if self.peek() == ",":
                self.eat(",")
            elif self.peek() != "}":
                raise ParseError("expected , or } at %d: %r" % (self.i, self.s[self.i:self.i + 40]))
