// In-memory filesystem for the Go js/wasm runtime.
//
// Go's syscall layer on js/wasm delegates every file operation to a global
// `fs` object with Node.js-style callback APIs (see $GOROOT/src/syscall/
// fs_js.go). This file provides that object backed by a plain in-memory
// tree, so the unmodified TypeFerence compiler can read sources and write
// artifacts entirely inside the browser tab. It must be loaded BEFORE
// wasm_exec.js, which only installs its non-functional stub when no `fs`
// global exists.
//
// Error objects carry a `code` property from Go's errnoByCode table
// (ENOENT, EEXIST, EISDIR, ...); Go panics on codes it does not know.
"use strict";
(() => {
  const decoder = new TextDecoder("utf-8");

  const S_IFDIR = 0o40000;
  const S_IFREG = 0o100000;

  const constants = {
    O_RDONLY: 0,
    O_WRONLY: 1,
    O_RDWR: 2,
    O_CREAT: 64,
    O_EXCL: 128,
    O_TRUNC: 512,
    O_APPEND: 1024,
    O_DIRECTORY: 65536,
  };

  let nextIno = 1;
  const makeDir = () => ({ dir: true, ino: nextIno++, entries: new Map(), mtimeMs: Date.now() });
  const makeFile = () => ({ dir: false, ino: nextIno++, data: new Uint8Array(0), mtimeMs: Date.now() });

  let root = makeDir();

  const errno = (code) => {
    const err = new Error(code);
    err.code = code;
    return err;
  };

  // "/a/./b/../c" -> ["a", "c"]; relative paths resolve from "/".
  const segmentsOf = (path) => {
    const out = [];
    for (const part of String(path).split("/")) {
      if (part === "" || part === ".") continue;
      if (part === "..") out.pop();
      else out.push(part);
    }
    return out;
  };

  const lookup = (path) => {
    let node = root;
    for (const segment of segmentsOf(path)) {
      if (!node.dir) return null;
      node = node.entries.get(segment);
      if (!node) return null;
    }
    return node;
  };

  // Returns [parentDirNode, finalName] or null when an ancestor is missing.
  const lookupParent = (path) => {
    const segments = segmentsOf(path);
    if (segments.length === 0) return null;
    let node = root;
    for (const segment of segments.slice(0, -1)) {
      if (!node.dir) return null;
      node = node.entries.get(segment);
      if (!node) return null;
    }
    if (!node.dir) return null;
    return [node, segments[segments.length - 1]];
  };

  const statOf = (node) => ({
    dev: 1,
    ino: node.ino,
    mode: (node.dir ? S_IFDIR : S_IFREG) | 0o644,
    nlink: 1,
    uid: 0,
    gid: 0,
    rdev: 0,
    size: node.dir ? 0 : node.data.length,
    blksize: 4096,
    blocks: node.dir ? 0 : Math.ceil(node.data.length / 512),
    atimeMs: node.mtimeMs,
    mtimeMs: node.mtimeMs,
    ctimeMs: node.mtimeMs,
    isDirectory() { return node.dir; },
    isFile() { return !node.dir; },
    isSymbolicLink() { return false; },
  });

  // Open file descriptors. 0/1/2 are reserved for the standard streams.
  const fds = new Map();
  let nextFd = 100;

  // Captured standard output, mirrored to the console line by line.
  const output = { stdout: "", stderr: "" };
  const lineBuf = { 1: "", 2: "" };
  const writeStream = (fd, bytes) => {
    const text = decoder.decode(bytes);
    if (fd === 1) output.stdout += text;
    else output.stderr += text;
    lineBuf[fd] += text;
    let nl;
    while ((nl = lineBuf[fd].indexOf("\n")) >= 0) {
      const line = lineBuf[fd].slice(0, nl);
      lineBuf[fd] = lineBuf[fd].slice(nl + 1);
      (fd === 1 ? console.log : console.error)(line);
    }
    return bytes.length;
  };

  const writeAt = (node, bytes, position) => {
    const end = position + bytes.length;
    if (end > node.data.length) {
      const grown = new Uint8Array(end);
      grown.set(node.data);
      node.data = grown;
    }
    node.data.set(bytes, position);
    node.mtimeMs = Date.now();
    return bytes.length;
  };

  globalThis.fs = {
    constants,

    writeSync(fd, buf) {
      if (fd === 1 || fd === 2) return writeStream(fd, buf);
      const handle = fds.get(fd);
      if (!handle || handle.node.dir) throw errno(fd in fds ? "EISDIR" : "EBADF");
      const n = writeAt(handle.node, buf, handle.pos);
      handle.pos += n;
      return n;
    },

    write(fd, buf, offset, length, position, callback) {
      const bytes = buf.subarray(offset, offset + length);
      if (fd === 1 || fd === 2) {
        callback(null, writeStream(fd, bytes));
        return;
      }
      const handle = fds.get(fd);
      if (!handle) return callback(errno("EBADF"));
      if (handle.node.dir) return callback(errno("EISDIR"));
      const at = position === null || position === undefined ? handle.pos : position;
      const n = writeAt(handle.node, bytes, at);
      if (position === null || position === undefined) handle.pos += n;
      callback(null, n);
    },

    read(fd, buffer, offset, length, position, callback) {
      const handle = fds.get(fd);
      if (!handle) return callback(errno("EBADF"));
      if (handle.node.dir) return callback(errno("EISDIR"));
      const at = position === null || position === undefined ? handle.pos : position;
      const available = Math.max(0, handle.node.data.length - at);
      const n = Math.min(length, available);
      buffer.set(handle.node.data.subarray(at, at + n), offset);
      if (position === null || position === undefined) handle.pos += n;
      callback(null, n);
    },

    open(path, flags, mode, callback) {
      let node = lookup(path);
      if (flags & constants.O_CREAT) {
        if (node && flags & constants.O_EXCL) return callback(errno("EEXIST"));
        if (!node) {
          const parent = lookupParent(path);
          if (!parent) return callback(errno("ENOENT"));
          node = makeFile();
          parent[0].entries.set(parent[1], node);
        }
      }
      if (!node) return callback(errno("ENOENT"));
      if (flags & constants.O_DIRECTORY && !node.dir) return callback(errno("ENOTDIR"));
      if (flags & constants.O_TRUNC && !node.dir) {
        node.data = new Uint8Array(0);
        node.mtimeMs = Date.now();
      }
      const fd = nextFd++;
      fds.set(fd, {
        node,
        pos: flags & constants.O_APPEND && !node.dir ? node.data.length : 0,
      });
      callback(null, fd);
    },

    close(fd, callback) {
      fds.delete(fd);
      callback(null);
    },

    stat(path, callback) {
      const node = lookup(path);
      if (!node) return callback(errno("ENOENT"));
      callback(null, statOf(node));
    },

    lstat(path, callback) {
      globalThis.fs.stat(path, callback);
    },

    fstat(fd, callback) {
      const handle = fds.get(fd);
      if (!handle) return callback(errno("EBADF"));
      callback(null, statOf(handle.node));
    },

    readdir(path, callback) {
      const node = lookup(path);
      if (!node) return callback(errno("ENOENT"));
      if (!node.dir) return callback(errno("ENOTDIR"));
      callback(null, [...node.entries.keys()]);
    },

    mkdir(path, perm, callback) {
      if (lookup(path)) return callback(errno("EEXIST"));
      const parent = lookupParent(path);
      if (!parent) return callback(errno("ENOENT"));
      parent[0].entries.set(parent[1], makeDir());
      callback(null);
    },

    rmdir(path, callback) {
      const parent = lookupParent(path);
      const node = parent && parent[0].entries.get(parent[1]);
      if (!node) return callback(errno("ENOENT"));
      if (!node.dir) return callback(errno("ENOTDIR"));
      if (node.entries.size > 0) return callback(errno("ENOTEMPTY"));
      parent[0].entries.delete(parent[1]);
      callback(null);
    },

    unlink(path, callback) {
      const parent = lookupParent(path);
      const node = parent && parent[0].entries.get(parent[1]);
      if (!node) return callback(errno("ENOENT"));
      if (node.dir) return callback(errno("EISDIR"));
      parent[0].entries.delete(parent[1]);
      callback(null);
    },

    rename(from, to, callback) {
      const fromParent = lookupParent(from);
      const node = fromParent && fromParent[0].entries.get(fromParent[1]);
      if (!node) return callback(errno("ENOENT"));
      const toParent = lookupParent(to);
      if (!toParent) return callback(errno("ENOENT"));
      fromParent[0].entries.delete(fromParent[1]);
      toParent[0].entries.set(toParent[1], node);
      callback(null);
    },

    truncate(path, length, callback) {
      const node = lookup(path);
      if (!node) return callback(errno("ENOENT"));
      if (node.dir) return callback(errno("EISDIR"));
      const data = new Uint8Array(length);
      data.set(node.data.subarray(0, Math.min(length, node.data.length)));
      node.data = data;
      node.mtimeMs = Date.now();
      callback(null);
    },

    ftruncate(fd, length, callback) {
      const handle = fds.get(fd);
      if (!handle) return callback(errno("EBADF"));
      if (handle.node.dir) return callback(errno("EISDIR"));
      const data = new Uint8Array(length);
      data.set(handle.node.data.subarray(0, Math.min(length, handle.node.data.length)));
      handle.node.data = data;
      handle.node.mtimeMs = Date.now();
      callback(null);
    },

    utimes(path, atime, mtime, callback) {
      const node = lookup(path);
      if (!node) return callback(errno("ENOENT"));
      node.mtimeMs = mtime * 1000;
      callback(null);
    },

    chmod(path, mode, callback) { callback(lookup(path) ? null : errno("ENOENT")); },
    fchmod(fd, mode, callback) { callback(fds.has(fd) ? null : errno("EBADF")); },
    chown(path, uid, gid, callback) { callback(lookup(path) ? null : errno("ENOENT")); },
    fchown(fd, uid, gid, callback) { callback(fds.has(fd) ? null : errno("EBADF")); },
    lchown(path, uid, gid, callback) { callback(lookup(path) ? null : errno("ENOENT")); },
    fsync(fd, callback) { callback(null); },
    link(from, to, callback) { callback(errno("EPERM")); },
    symlink(from, to, callback) { callback(errno("EPERM")); },
    readlink(path, callback) { callback(errno("EINVAL")); },
  };

  // syscall.Open resolves every opened path through this.
  globalThis.path = {
    resolve(...parts) {
      return "/" + segmentsOf(parts.join("/")).join("/");
    },
  };

  // Test and debugging surface; the playground UI reads captured output.
  globalThis.__memfs = {
    output,
    reset() {
      root = makeDir();
      fds.clear();
      output.stdout = "";
      output.stderr = "";
      lineBuf[1] = "";
      lineBuf[2] = "";
    },
  };
})();
