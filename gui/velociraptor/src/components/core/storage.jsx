/*
  Manage session storage for the application.
*/

export const setItem = (key, value)=>{
    sessionStorage.setItem(key, value);
};

export const getItem = key=>{
    let res = sessionStorage.getItem(key);
    let decoder = decoders[key];
    if(decoder) {
        res = decoder(res);
    }
    return res;
};

const decoders = {
    CurrentPageSize: x=>parseInt(x),
};

// This is effectively an application global state that exists within
// the same browser tab. The state is visible across from all
// components so it kind of breaks encapsulation a bit but we have a
// lot of major components which are effectively global.
export let schema = {
    // Global
    CurrentSelectedClientKey: "CurrentSelectedClient",

    // Hunt Pane
    CurrentHuntIdKey: "HuntsCurrentSelected",
    CurrentHuntTabKey: "CurrentHuntTab",
    HuntSplitKey: "HuntSplitSize",

    // Artifact Viewer
    CurrentSelectedArtifactKey: "CurrentSelectedArtifact",
    ArtifactSplitKey: "ArtifactsSplitSize",

    // Server Event Viewer
    CurrentServerEventArtifactKey: "CurrentServerEventArtifact",

    // Client Event Viewer
    CurrentClientEventArtifactKey: "CurrentClientEventArtifact",

    // Client Flows Viewer
    ClientsSplitKey: "ClientsSplitSize",
    ClientsCurrentFlowKey: "ClientsCurrentFlow",
    ClientFlowsTabKey: "ClientFlowsTab",

    // Server Flows Viewer
    ServerSplitKey: "ServerSplitSize",
    ServerCurrentFlowKey: "ServerCurrentFlow",

    // Notebook
    CurrentNotebookIdKey: "CurrentNotebookId",

    // VFS
    CurrentVFSPathKey: "CurrentVFSPath",

    // Table view
    CurrentPageSizeKey: "CurrentPageSize",
};
