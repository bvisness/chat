import { Client } from "./ui";

// @ts-ignore
const clients: HTMLDivElement = window.clients;
//@ts-ignore
const container1: HTMLDivElement = window.one;
//@ts-ignore
const container2: HTMLDivElement = window.two;

const client1 = new Client();
const client2 = new Client();

container1.replaceWith(client1.container);
container2.replaceWith(client2.container);

client1.connect();
setTimeout(() => client2.connect(), 500);
