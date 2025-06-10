/**
 * @jest-environment jsdom
 * @jest-environment-options {"url": "https://www.example.com/prefix/velociraptor/app/index.html"}
 */

import api from '../core/api-service.jsx';

/*
  Test the href() function.

  This function constructs a URL based on the current SPA URL and/or
  the base path reported by the server. This function makes it
  possible to host Velociraptor on very unusual deployments (e.g. with
  rewriting reverse proxies, CDN URLs etc).
*/

test('test href with empty base_path, empty org id', () => {
    window.base_path = "";
    window.globals = {};

    // If base_path is "" we take the base path from the current URL
    expect(api.base_path()).toBe("/prefix/velociraptor");

    expect(api.href("/api/v1/DownloadVFSFile")).toBe(
        "https://www.example.com/prefix/velociraptor/api/v1/DownloadVFSFile?org_id=root");
});


test('test href with empty base_path, specified org id', () => {
    window.base_path = "";
    window.globals = {
        OrgId: "O123"
    };

    expect(api.href("/api/v1/DownloadVFSFile")).toBe(
        "https://www.example.com/prefix/velociraptor/api/v1/DownloadVFSFile?org_id=O123");
});


test('test href with new base_path, specified org id', () => {
    window.base_path = "/new/base_path/";
    window.globals = {
        OrgId: "O123"
    };

    expect(api.base_path()).toBe("/new/base_path/");

    expect(api.href("/api/v1/DownloadVFSFile")).toBe(
        "https://www.example.com/new/base_path/api/v1/DownloadVFSFile?org_id=O123");
});


// If base path is / it means to ignore the current location and force
// it to be at the top level.
test('test href with / base_path, specified org id', () => {
    window.base_path = "/";
    window.globals = {
        OrgId: "O123"
    };

    expect(api.base_path()).toBe("/");
    expect(api.href("/api/v1/DownloadVFSFile")).toBe(
        "https://www.example.com/api/v1/DownloadVFSFile?org_id=O123");
});


// An unrelated link is not changed.
test('test unrelated external link', () => {
    window.base_path = "/";
    window.globals = {
        OrgId: "O123"
    };

    // Should be left alone
    expect(api.href("https://www.example.com/third/party/app/link.html")).toBe(
        "https://www.example.com/third/party/app/link.html");
});

test('An internal link to the SPA', () => {
    window.base_path = "";
    window.globals = {
        OrgId: "O123"
    };

    // Retain the prefix and org id properly. Propagate the fragment for SPA navigation.
    expect(api.href("/app#/collected/C.24232b1c7c412020/")).toBe(
        "https://www.example.com/prefix/velociraptor/app/index.html?org_id=O123#/collected/C.24232b1c7c412020/");
});

test('An internal link to the SPA', () => {
    window.base_path = "";
    window.globals = {
        OrgId: "O123"
    };

    // An example internal link prepared by the Go link_to() function.
    const url = "https://www.example.com/prefix/velociraptor/api/v1/DownloadVFSFile?fs_components=clients&fs_components=C.24232b1c7c412020&fs_components=collections&fs_components=test.txt&org_id=root&vfs_path=test.txt";

    // Retain the prefix and org id properly. Propagate the fragment for SPA navigation.
    expect(api.href(url)).toBe(url);
});
