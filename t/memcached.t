#!/usr/bin/env perl
# vim: set sw=4 ts=4:
use strict;
use warnings;
use FindBin;
use Test::More;
use File::Temp qw(tempfile);
use Test::TCP;
use Net::EmptyPort qw(empty_port);
use Cache::Memcached;


my $memcached = Test::TCP->new(
    code => sub {
        my $port = shift;
        exec('memcached', '-vv', '-p', $port);
    });
ok($memcached, 'memcached started');

my $wsgate_server = Test::TCP->new(
    code => sub {
        my $port = shift;

        my ($fh, $server_map) = tempfile();
        print $fh "# memcached\n";
        print $fh sprintf("memcached,127.0.0.1:%d\n", $memcached->port);
        close($fh);

        exec("wsgate-server", '-map', $server_map, '-public-key', "$FindBin::Bin/../sample-public.pem", "-listen", "127.0.0.1:$port");
    });
ok($wsgate_server, 'wsgate-server started');


my $wsgate_client = Test::TCP->new(
    code => sub {
        my $port = shift;

        my ($fh, $client_map) = tempfile();
        print $fh "# memcached\n";
        print $fh sprintf("127.0.0.1:%d,http://127.0.0.1:%d/proxy/memcached\n", $port, $wsgate_server->port);
        close($fh);

        exec("$FindBin::Bin/../wsgate-client", '--map', $client_map, '--private-key', "$FindBin::Bin/../sample-secret.pem");
    });
ok($wsgate_client, 'wsgate-client started');

my $memcached_client = Cache::Memcached->new(
    servers => ['127.0.0.1:'.$wsgate_client->port],
);

ok($memcached_client, 'memcached client created');
ok($memcached_client->set('key', 'value'), 'set key');
is($memcached_client->get('key'), 'value', 'get key');

undef $memcached;
undef $wsgate_server;
undef $wsgate_client;

done_testing();