#include <assert.h>
#include <errno.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <arpa/inet.h>
#include <netinet/ip.h>
#include <string.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <unistd.h>
#include <pwd.h>

#define SERVER_ADDR "47.100.31.117"
#define SERVER_PORT 20018

int udpClient(const char *server_addr, int server_port)
{
	int sock = socket(AF_INET, SOCK_DGRAM, 0);
	if (sock < 0)
	{
		printf("create socket failed\n");
		return 1;
	}

	sockaddr_in myaddr = {0};
	myaddr.sin_port = htons(rand() % 800 + 9001);
	myaddr.sin_family = AF_INET;
	myaddr.sin_addr.s_addr = INADDR_ANY;

	int val = 1;
	int ret = setsockopt(sock, SOL_SOCKET, SO_REUSEADDR, (const char *)&val, sizeof(val));
	if (ret == -1)
	{
		printf("setsockopt failed error:%d\n", errno);
		close(sock);
		return 2;
	}

	ret = bind(sock, (sockaddr *)&myaddr, sizeof(sockaddr_in));
	if (ret == -1)
	{
		printf("bind failed error:%d\n", errno);
		close(sock);
		return 2;
	}
	sockaddr_in servAddr = {0};
	servAddr.sin_port = htons(server_port);
	servAddr.sin_family = AF_INET;
	servAddr.sin_addr.s_addr = inet_addr(server_addr);
	// servAddr.sin_addr.S_un.S_addr = inet_addr(server_addr);
	char buf[0x40] = {0};

	ret = sendto(sock, buf, 0x10, 0, (sockaddr *)&servAddr, sizeof(sockaddr_in));

	sockaddr_in recvAddr = {0};
	socklen_t addrLen = sizeof(sockaddr_in);
	printf("wait recv peer addr...\n");
	ret = recvfrom(sock, buf, 12, 0, (sockaddr *)&recvAddr, &addrLen);
	if (ret == -1)
	{
		printf("recv failed  error:%d\n", errno);
		close(sock);
		return 3;
	}

	printf("recv from: %s:%d\n", inet_ntoa(recvAddr.sin_addr), ntohs(recvAddr.sin_port));

	sockaddr_in peerAddr = {0};
	peerAddr.sin_family = AF_INET;
	peerAddr.sin_addr.s_addr = *(int *)buf;
	// peerAddr.sin_addr.s_addr = 0x7B93DF7B;
	peerAddr.sin_port = *(short *)&buf[4];
	struct in_addr selfip = {0};
	selfip.s_addr = *(int *)&buf[6];
	short selfPort = *(short *)&buf[10];
	selfPort = ntohs(selfPort);
	char strIp[0x10] = {0};
	strcpy(strIp, inet_ntoa(selfip));
	printf("recv data: myself(%s:%d)  peer(%s:%d)\n",
		   strIp, selfPort,
		   inet_ntoa(peerAddr.sin_addr), ntohs(peerAddr.sin_port));
	if (peerAddr.sin_addr.s_addr == selfip.s_addr)
	{
		printf("back of the same NAT, no need NAT hole\n");
		close(sock);
		return 4;
	}
	struct timeval timeout = {0, 300000}; // 300ms
	setsockopt(sock, SOL_SOCKET, SO_RCVTIMEO, (char *)&timeout, sizeof(struct timeval));
	char msg[0x40] = "0123456";
	struct passwd *pwd = getpwuid(getuid());
	sprintf(msg, "p2p->(%d:%s) say hello", getpid(), pwd->pw_name);
	printf("send peer msg: %s\n", msg);
	printf("wait peer response\n");

	for (int i = 0; i < 5; ++i)
	{
		ret = sendto(sock, msg, strlen(msg) + 1, 0, (sockaddr *)&peerAddr, sizeof(sockaddr_in));
		addrLen = sizeof(sockaddr_in);
		ret = recvfrom(sock, buf, 0x40, 0, (sockaddr *)&recvAddr, &addrLen);
		if (ret >= 0)
		{
			break;
		}
	}
	if (ret < 1)
	{
		printf("udp hole failed!! errno:%d\n", errno);
		close(sock);
		return 0;
	}
	printf("recv:%d, from:%s:%d, data:%s\n", ret, inet_ntoa(recvAddr.sin_addr), ntohs(recvAddr.sin_port), buf);
	ret = sendto(sock, msg, strlen(msg) + 1, 0, (sockaddr *)&recvAddr, sizeof(sockaddr_in));
	sleep(1);

	sprintf(msg, "p2p->(%d:%s) say byebye!", getpid(), pwd->pw_name);
	ret = sendto(sock, msg, strlen(msg) + 1, 0, (sockaddr *)&recvAddr, sizeof(sockaddr_in));
	sleep(1);

	ret = recvfrom(sock, buf, 0x40, 0, (sockaddr *)&recvAddr, &addrLen);
	if (ret > 0)
	{
		printf("recv:%d, from:%s:%d, data:%s\n", ret, inet_ntoa(recvAddr.sin_addr), ntohs(recvAddr.sin_port), buf);
	}
	sleep(2);

	close(sock);
	return 0;
}

int main()
{
	return udpClient(SERVER_ADDR, SERVER_PORT);
}
