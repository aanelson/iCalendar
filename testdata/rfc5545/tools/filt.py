import sys,re
txt=open(sys.argv[1]).read().split("=== ")[1:]
keep=[]
for b in txt:
    lines=b.rstrip("\n").split("\n")
    body=[l.split("|",1)[1] for l in lines[1:] if "|" in l]
    abnf=sum(1 for l in body if re.match(r"^\s*[a-zA-Z][\w-]*\s+=(\s|$)",l) or re.match(r"^\s*;",l) or re.match(r"^\s*/\s",l) or re.search(r'"\s*/\s*"|\)\s*$|^\s*\*\(',l))
    if abnf and abnf*1.0/len(body) > 0.2:
        continue
    keep.append("=== "+b)
open(sys.argv[2],"w").write("".join(keep))
print(len(keep))
