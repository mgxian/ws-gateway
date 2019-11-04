// anonymous user
const socket = new WebSocket('ws://localhost:5000/');
socket.addEventListener('open', function (event) {
    socket.send('{"member_id": -1, "token": ""}');
    socket.send('{"app": "match"}')
    socket.send('{"app": "im"}')
    socket.send('{"app": "chat"}')
});
socket.addEventListener('message', function (event) {
    message = JSON.parse(event.data)
    if (message.text) {
        console.log('Message from server ', JSON.parse(message.text));
        return
    }
    console.log('Message from server ', JSON.parse(event.data));
});

// valid user
const socket = new WebSocket('ws://localhost:5000/');
socket.addEventListener('open', function (event) {
    socket.send('{"member_id": 123456, "token": "654321"}');
    socket.send('{"app": "match"}')
    socket.send('{"app": "im"}')
    socket.send('{"app": "chat"}')
});
socket.addEventListener('message', function (event) {
    message = JSON.parse(event.data)
    if (message.text) {
        console.log('Message from server ', JSON.parse(message.text));
        return
    }
    console.log('Message from server ', JSON.parse(event.data));
});

// invalid user
const socket = new WebSocket('ws://localhost:5000/');
socket.addEventListener('open', function (event) {
    socket.send('{"member_id": 12345, "token": "54321"}');
    socket.send('{"app": "match"}')
    socket.send('{"app": "im"}')
    socket.send('{"app": "chat"}')
});
socket.addEventListener('message', function (event) {
    message = JSON.parse(event.data)
    if (message.text) {
        console.log('Message from server ', JSON.parse(message.text));
        return
    }
    console.log('Message from server ', JSON.parse(event.data));
});


// test for httpie
// http POST http://localhost:5000/push app=match member_id:=-1 text='{"hello":"world"}'
// http POST http://localhost:5000/push app=im member_id:=123456 text='{"hello":123456}'