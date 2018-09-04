# Event Receiver

This application receives nunchuk input events that were send by [**Event Sender**] over the network.

It is part of the _BeagleBone Black_ [buildroot project].

## Functionality
Button input events (**Button C** and **Button Z**) and Joystick input events (**Joystick-Axis X** and **Joystick-Axis Y**) are received as _UDP_ packets from a remote computer.

Events are packed into a [protobuf], that is described by `nunchuk_update.proto`.

IP and Port of the receiving service are advertised by [avahi]. (TODO)


[//]: # (Reference Links)
[avahi]: <https://www.avahi.org/>
[protobuf]: <https://developers.google.com/protocol-buffers/>

[buildroot project]: <https://bitbucket.org/MarcoHartmann/buildroot_bbb/src>
[**Event Sender**]: <https://bitbucket.org/MarcoHartmann/event_sender/src/master/>
