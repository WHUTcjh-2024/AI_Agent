"""Compatibility entry point. In-place legacy cleaning has been retired.

Requires explicit input and a new output batch; old --all/--fix shortcuts fail closed.
"""
from verify_clean_batch import main

if __name__ == "__main__":
    main()
