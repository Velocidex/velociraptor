#ifndef WIN32_LEAN_AND_MEAN
#define WIN32_LEAN_AND_MEAN
#endif

// Link to ws2_32.lib
#include <winsock2.h>
#include <ws2tcpip.h>
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>

#define MAX_PACKET            65535

void process_dns(void *ctx, char *buff, int length);

struct iphdr {
    uint8_t ihl:4;
    uint8_t version:4;
    uint8_t tos;
    uint16_t tot_len;
    uint16_t id;
    uint16_t frag_off;
    uint8_t ttl;
    uint8_t protocol;
    uint16_t checksum;
    uint32_t saddr;
    uint32_t daddr;
};

struct udphdr {
    uint16_t source;
    uint16_t dest;
    uint16_t len;
    uint16_t checksum;
};


// Accounting we need on the C side.
typedef struct  {
    void *go_ctx;

    // The main raw socket we read DNS packets on.
    SOCKET s;
    int running;

    // Hold onto the promisc sockets so we can close them at shutdown.
    SOCKET promisc[50];
    int number_of_promisc;
} watcher_context;


// Sets all the interfaces into promiscuous mode. We can chek like
// this: Get-NetAdapter | Format-List -Property PromiscuousMode
//
// We only need to set any socket on the interface as promiscuous to
// allow all packets to reach any of the listening sockets. We dont
// actually read from this socket so we do not have to filter through
// all the uninteresting packets.
// https://msdn.microsoft.com/en-us/library/windows/desktop/ee309610(v=vs.85).aspx
void setPromiscuous(watcher_context *ctx) {
    int rc;
    SOCKADDR_IN   if0;
    SOCKET_ADDRESS_LIST *slist=NULL;
    char                 buf[2048];
    DWORD                dwBytesRet;
    int i;
    int promisc_idx = 0;

    ctx->number_of_promisc = 0;

    // Create the raw socket
    ctx->promisc[promisc_idx] = socket(AF_INET, SOCK_RAW, IPPROTO_IP);
    if (ctx->promisc[promisc_idx] == INVALID_SOCKET) {
        goto error;
    }
    ctx->number_of_promisc++;

    rc = WSAIoctl(ctx->promisc[promisc_idx],
                   SIO_ADDRESS_LIST_QUERY, NULL, 0, buf,
                   sizeof(buf), &dwBytesRet, NULL, NULL);
    if (rc == SOCKET_ERROR) {
        return;
    }

    slist = (SOCKET_ADDRESS_LIST *)buf;
    for (i=0; i<slist->iAddressCount; i++) {
        if0.sin_addr.s_addr = ((SOCKADDR_IN *)slist->Address[i].
                               lpSockaddr)->sin_addr.s_addr;
        if0.sin_family = AF_INET;
        if0.sin_port = htons(0);

        promisc_idx ++;
        if (promisc_idx > sizeof(ctx->promisc)) {
            return;
        }

        ctx->promisc[promisc_idx] = socket(
            AF_INET, SOCK_RAW, IPPROTO_IP);
        if (ctx->promisc[promisc_idx] == INVALID_SOCKET) {
            goto error;
        }
        ctx->number_of_promisc++;

        rc = bind(ctx->promisc[promisc_idx],
                  (SOCKADDR *)&if0, sizeof(if0));
        if (rc == SOCKET_ERROR) {
            continue;
        }

        DWORD flag = RCVALL_ON ;
        DWORD ret = 0;
        rc = WSAIoctl(ctx->promisc[promisc_idx],
                      SIO_RCVALL, &flag, sizeof(flag),
                      NULL, 0,  &ret, NULL, NULL);
    }

 error:
    // Cant really do anything in the case of failure - just move on.
    return;
}


// These functions are called from Go to create and destroy an event
// watcher context.
void *watchDNS(void *go_ctx) {
    watcher_context *ctx = (watcher_context *)calloc(sizeof(watcher_context), 1);
    WSADATA            wsd;
    struct addrinfo   *dest=NULL, *local=NULL;
    int rc;
    struct sockaddr_in   if0;

    // Create a new C context
    ctx->go_ctx = go_ctx;
    ctx->running = 1;

    // Load Winsock
    if (WSAStartup(MAKEWORD(2,2), &wsd) != 0) {
        goto error;
    }

    // Set all interfaces to be promiscuous.
    setPromiscuous(ctx);

    // Create the raw socket for UDP
    ctx->s = socket(AF_INET, SOCK_RAW, IPPROTO_UDP);
    if (ctx->s == INVALID_SOCKET) {
        goto error;
    }

    // We only care about DNS
    if0.sin_family = AF_INET;
    if0.sin_addr.s_addr = INADDR_ANY;
    if0.sin_port = htons(53);

    rc = bind(ctx->s, (SOCKADDR *)&if0, sizeof(if0));
    if (rc == SOCKET_ERROR) {
        closesocket(ctx->s);
        goto error;
    }

    return ctx;

 error:
    return NULL;
}

void destroyDNS(void *c_ctx) {
    watcher_context *ctx = (watcher_context *)c_ctx;
    int i;

    // Close all the sockets.
    closesocket(ctx->s);
    for(i=0; i<ctx->number_of_promisc;i++) {
        closesocket(ctx->promisc[i]);
    };

    ctx->running = 0;
}


void runDNS(void *c_ctx) {
    watcher_context *ctx = (watcher_context *)c_ctx;
    unsigned char buf[MAX_PACKET];

    // Post the first overlapped receive
    for (;;) {
        SOCKADDR_STORAGE    safrom;
        int fromlen = sizeof(safrom);
        int rc;

        if (!ctx->running) {
            goto exit;
        }

        rc =  recvfrom(ctx->s, buf, MAX_PACKET, 0, (SOCKADDR *)&safrom, &fromlen);
        // Wait for a response
        if (rc == SOCKET_ERROR) {
            goto exit;

        } else {
            if (rc > 28) {
                struct iphdr *ip_header = (struct iphdr *)buf;
                int iphdr_size = ip_header->ihl * 4;
                struct udphdr *udp_header = (struct udphdr *)(buf + iphdr_size);

                // We only care about DNS packets.
                if (ntohs(udp_header->source) == 53 ||
                    ntohs(udp_header->dest) == 53) {
                    int start_of_dns = iphdr_size + sizeof(struct udphdr);
                    if (start_of_dns < rc) {
                        process_dns(ctx->go_ctx,
                                    buf + start_of_dns,
                                    rc - start_of_dns);
                    }
                }
            }
        }
    }

 exit:
    closesocket(ctx->s);
    free(c_ctx);
}
