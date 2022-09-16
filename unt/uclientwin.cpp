#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include <assert.h>
#include <errno.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <WinSock2.h>
#include <ws2tcpip.h>
#pragma comment(lib, "ws2_32.lib")

#pragma comment(lib, "Advapi32.lib")

#include <udt.h>

class CAutoSockInit
{
public:
    CAutoSockInit()
    {
        UDT::startup();
    }
    ~CAutoSockInit()
    {
        UDT::cleanup();
    }
};

int udpClient(const char *server_addr, int server_port)
{
    CAutoSockInit autoinit;

    int sock = socket(AF_INET, SOCK_DGRAM, 0);
    if (sock < 0)
    {
        printf("create socket failed\n");
        return 0;
    }

    srand(GetTickCount());

    sockaddr_in myaddr = {0};
    myaddr.sin_port = htons(rand() % 800 + 9001);
    myaddr.sin_family = AF_INET;
    myaddr.sin_addr.S_un.S_addr = INADDR_ANY;
    int ret = 0;

    int val = 1;
    setsockopt(sock, SOL_SOCKET, SO_REUSEADDR, (char *)val, sizeof(val));

    ret = bind(sock, (sockaddr *)&myaddr, sizeof(sockaddr_in));
    if (ret == -1)
    {
        printf("bind failed  error:%d", WSAGetLastError());
        closesocket(sock);
        return 0;
    }
    sockaddr_in servAddr = {0};
    servAddr.sin_port = htons(server_port);
    servAddr.sin_family = AF_INET;
    servAddr.sin_addr.S_un.S_addr = inet_addr(server_addr);
    // servAddr.sin_addr.S_un.S_addr = inet_addr(server_addr);

    char buf[0x20] = {0};

    ret = sendto(sock, buf, 0x10, 0, (sockaddr *)&servAddr, sizeof(sockaddr_in));

    sockaddr_in recvAddr = {0};
    int addrLen = sizeof(sockaddr_in);
    printf("wait recv peer addr...\n");
    ret = recvfrom(sock, buf, 12, 0, (sockaddr *)&recvAddr, &addrLen);
    if (ret == -1)
    {
        printf("recv failed  error:%d", WSAGetLastError());
        closesocket(sock);
        return 0;
    }

    printf("recv from: %s:%d\n", inet_ntoa(recvAddr.sin_addr), ntohs(recvAddr.sin_port));

    sockaddr_in peerAddr = {0};
    peerAddr.sin_family = AF_INET;
    peerAddr.sin_addr.S_un.S_addr = *(int *)buf;
    peerAddr.sin_port = *(short *)&buf[4];

    IN_ADDR selfIp = {0};
    selfIp.S_un.S_addr = *(int *)&buf[6];
    short port = *(short *)&buf[10];

    char sIP[0x20] = {0};
    strcpy_s(sIP, inet_ntoa(selfIp));
    printf("recv data: my( %s:%d) peer( %s:%d ) \n",
           sIP, ntohs(port),
           inet_ntoa(peerAddr.sin_addr), ntohs(peerAddr.sin_port));

    if (peerAddr.sin_addr.S_un.S_addr == selfIp.S_un.S_addr)
    {
        printf("no need NAT hole,  you and peer in the back of same NAT\n");
        closesocket(sock);
        system("pause");
        return 0;
    }

    char userName[0x20] = {0};
    DWORD len = 0x20;
    GetUserName(userName, &len);
    char msg[0x20] = "0123456";
    sprintf_s(msg, 0x20, "%d:[%s] hello", GetCurrentThreadId(), userName);
    printf("send msg to peer: %s \n", msg);
    printf("wait  peer back\n");
    int to = 500;
    ret = setsockopt(sock, SOL_SOCKET, SO_RCVTIMEO, (char *)&to, sizeof(to));
    for (int i = 0; i < 5; ++i)
    {
        ret = sendto(sock, msg, strlen(msg) + 1, 0, (sockaddr *)&peerAddr, sizeof(sockaddr_in));
        addrLen = sizeof(sockaddr_in);
        ret = recvfrom(sock, buf, 0x20, 0, (sockaddr *)&recvAddr, &addrLen);
        if (ret >= 0)
        {
            printf("break ret:%d  error:%d\n", ret, WSAGetLastError());
            break;
        }
        printf("%d try  ret:%d  error:%d\n", i, ret, WSAGetLastError());
    }
    if (ret < 1)
    {
        printf("udp hole failed error:%d", WSAGetLastError());
        closesocket(sock);
        return 0;
    }
    printf("recv from: %s:%d\n", inet_ntoa(recvAddr.sin_addr), ntohs(recvAddr.sin_port));
    printf("data: %s\n", buf);
    ret = sendto(sock, msg, strlen(msg) + 1, 0, (sockaddr *)&recvAddr, sizeof(sockaddr_in));
    system("pause");
    closesocket(sock);

    return 0;
}
