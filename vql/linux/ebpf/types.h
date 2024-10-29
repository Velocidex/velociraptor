/* Kernel types we actually use.

   This means we dont need to load and ship the massive vmlinux.h
*/

typedef __u8 u8;
typedef __s16 s16;
typedef __u16 u16;
typedef __s32 s32;
typedef __u32 u32;
typedef __s64 s64;
typedef __u64 u64;

struct qstr {
    union {
        struct {
            u32 hash;
            u32 len;
        };
        u64 hash_len;
    };
    const unsigned char *name;
} __attribute__((preserve_access_index));

struct dentry {
    struct dentry *d_parent;
    struct qstr d_name;
} __attribute__((preserve_access_index));

struct vfsmount {
    struct dentry *mnt_root;
} __attribute__((preserve_access_index));

struct mount {
    struct mount *mnt_parent;
    struct dentry *mnt_mountpoint;
    struct vfsmount mnt;
} __attribute__((preserve_access_index));

struct path {
    struct vfsmount *mnt;
    struct dentry *dentry;
} __attribute__((preserve_access_index));

struct file {
    struct path f_path;
} __attribute__((preserve_access_index));

struct file___old {
    struct path path;
} __attribute__((preserve_access_index));

struct mm_struct {
    struct file *exe_file;
} __attribute__((preserve_access_index));

struct task_struct {
    int pid;
    struct task_struct *real_parent;
    char comm[16];
    struct mm_struct *mm;
    struct task_struct *group_leader;
} __attribute__((preserve_access_index));
