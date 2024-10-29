
//go:build ignore

#include <linux/bpf.h>
#include "types.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>

#define MAX_PATH 255
#define MAX_COMPONENTS 48

struct velo_proc_event {
    __u64 ktime;
    __u32 pid;
    __u32 ppid;
    __u8 exe_path[MAX_PATH];
    __u8 exe[MAX_PATH];
    __u8 comm[100];
    __u8 parent_comm[100];
};

const struct velo_proc_event *unused __attribute__((unused));

#define LIMIT_PATH_SIZE(x) ((x) & (MAX_PATH - 1))
#define MAX_PERCPU_ARRAY_SIZE 256
#define HALF_PERCPU_ARRAY_SIZE (MAX_PERCPU_ARRAY_SIZE >> 1)
#define LIMIT_PERCPU_ARRAY_SIZE(x) ((x) & (MAX_PERCPU_ARRAY_SIZE - 1))
#define LIMIT_HALF_PERCPU_ARRAY_SIZE(x) ((x) & (HALF_PERCPU_ARRAY_SIZE - 1))

struct buffer {
    u8 data[MAX_PERCPU_ARRAY_SIZE];
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    // Must be power of 2 multiple
    __uint(max_entries, 4096 * 256);
} events SEC(".maps");


int get_full_path(struct dentry *dentry, struct buffer *buf) {
    unsigned int out_i = 0;

    for(int i=0;i<MAX_COMPONENTS;i++) {
        struct dentry *parent = BPF_CORE_READ(dentry, d_parent);
        if (parent == 0) {
            break;
        }

        const __u8 *name = BPF_CORE_READ(dentry, d_name.name);
        if (name != 0) {
            int avail_size = sizeof(buf->data) - out_i;
            if (avail_size < 0) {
                break;
            }

            if (out_i >=  sizeof(buf->data)) {
                break;
            }

            int name_len = LIMIT_PATH_SIZE(BPF_CORE_READ(dentry, d_name.len));
            int length = bpf_probe_read_kernel_str(
                &(buf->data[LIMIT_HALF_PERCPU_ARRAY_SIZE(out_i)]),
                  name_len, name);
            if (length < 0) {
                break;
            }

            if (length >= 0) {
                out_i += length;
            }

            if (out_i > MAX_PATH) {
                break;
            }
        }
        dentry = parent;
    }
    return 0;
}

/*
int get_full_path(struct dentry *dentry, unsigned char *output, int size) {
    int component_idx, component_len, output_idx = 0;
    struct dentry *components[MAX_COMPONENTS];

    __builtin_memset(&components, 0, sizeof(components));

    for(component_idx=0;component_idx<MAX_COMPONENTS;component_idx++) {
        components[component_idx] = dentry;

        struct dentry *parent = BPF_CORE_READ(dentry, d_parent);
        if (parent == 0) {
            break;
        }

        dentry = parent;
    }

    component_len = component_idx;

    // Build the path components in reverse.
    for (component_idx = component_len; component_idx>=0; component_idx--) {
        if (output == 0) {
            break;
        }

        //output[output_idx] = '/';
        output_idx++;

        const int max_name_len = sizeof(dentry->d_name.name);
        struct dentry *local_dentry = 0;

        local_dentry = components[component_idx];
        if (local_dentry == 0) {
            continue;
        }

        const __u8 *name = BPF_CORE_READ(local_dentry, d_name.name);
        unsigned char tmp[sizeof(dentry->d_name.name)];

        if (name != 0) {
            bpf_probe_read_kernel_str(&tmp, sizeof(tmp), name);
            for (int i=0; i<max_name_len && output_idx < size; i++) {
                //output[output_idx] = name[i];
                if (name[i] == 0) {
                    break;
                }
                output_idx++;
            }
        }
    }

    return 0;
}
*/


SEC("kprobe/sys_execve")
int hook_execve(struct pt_regs *ctx) {
    struct velo_proc_event *e;
    e = bpf_ringbuf_reserve(&events, sizeof(struct velo_proc_event), 0);
    if (!e) {
        return 0;
    }

    e->ktime = bpf_ktime_get_ns();
    e->pid = (__u32)(bpf_get_current_pid_tgid() & 0xFFFFFFFF);

    struct task_struct *current_task =
        (struct task_struct *)bpf_get_current_task();

    struct task_struct *parent_task = 0;

    if(bpf_core_read(&parent_task, sizeof(void *),
                     &current_task->real_parent) == 0) {
        bpf_core_read(&e->ppid, sizeof(__u32), &parent_task->pid);
        bpf_probe_read_kernel_str(
            &e->parent_comm, sizeof(e->parent_comm), &parent_task->comm);
    }

    bpf_core_read_str(&e->comm, sizeof(e->comm), &current_task->comm);

    // Newer kernels rename path to f_path so handle both cases here.
    const struct file *exe_file = BPF_CORE_READ(current_task, mm, exe_file);
    if (exe_file != 0) {
        struct dentry *dentry;
        struct vfsmount *mnt;

        if (bpf_core_field_exists(exe_file->f_path)) {
            dentry = BPF_CORE_READ(exe_file, f_path.dentry);
            mnt = BPF_CORE_READ(exe_file, f_path.mnt);

        } else {
            struct file___old *old_exe_file = (struct file___old *)exe_file;
            dentry = BPF_CORE_READ(old_exe_file, path.dentry);
            mnt = BPF_CORE_READ(old_exe_file, path.mnt);
        }

        if (dentry != 0) {
            struct buffer buf;
            get_full_path(dentry, &buf);

            /*
            const __u8 *exe;
            exe = BPF_CORE_READ(dentry, d_name.name);
            if (exe != 0) {
                bpf_probe_read_kernel_str(&e->exe, sizeof(e->exe), exe);
            }

            if (mnt != 0) {
                struct mount *real_mount = container_of(mnt, struct mount, mnt);
                if (real_mount != 0) {
                    struct dentry *mnt_mountpoint =
                        BPF_CORE_READ(real_mount, mnt_mountpoint);

                    if (mnt_mountpoint != 0) {
                        exe = BPF_CORE_READ(mnt_mountpoint, d_name.name);
                        if (exe != 0) {
                            bpf_probe_read_kernel_str(
                                &e->exe_path, sizeof(e->exe_path), exe);
                        }
                    }

                if (exe_path != 0) {
                    bpf_probe_read_kernel_str(
                        &e->exe_path, sizeof(e->exe_path), exe_path);
                }
                */
        }
    }

    bpf_ringbuf_submit(e, 0);

    return 0;
}

char __license[] SEC("license") = "Dual MIT/GPL";
