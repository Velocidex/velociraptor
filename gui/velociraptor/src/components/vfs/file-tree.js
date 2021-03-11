import './file-tree.css';

import React, { Component } from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';
import axios from 'axios';
import {Treebeard, decorators} from 'react-treebeard';

import { SplitPathComponents, Join } from '../utils/paths.js';

import api from '../core/api-service.js';
import { withRouter }  from "react-router-dom";

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import { EncodePathInURL, DecodePathInURL } from '../utils/paths.js';

const text_color = "#8f8f8f";
const background_color = '#f5f5f5';
const active_background_color = "#dee0ff";


const Header = ({onSelect, style, customStyles, node}) => {
    return (
        <div style={style.base} onClick={onSelect}>
          <div className="tree-folder">
            {node.toggled ? <FontAwesomeIcon icon="folder-open" /> : <FontAwesomeIcon icon="folder" /> }
            {node.name}
          </div>
        </div>
    );
};

Header.propTypes = {
    onSelect: PropTypes.func,
    node: PropTypes.object,
    style: PropTypes.object,
    customStyles: PropTypes.object
};

Header.defaultProps = {
    customStyles: {}
};

let theme = {
  tree: {
    base: {
      listStyle: 'none',
      backgroundColor: background_color,
      margin: 0,
      padding: 0,
      color: text_color,
      fontFamily: 'lucida grande ,tahoma,verdana,arial,sans-serif',
      fontSize: '14px',
        marginLeft: '-20px',
        marginTop: '-20px',
    },
    node: {
      base: {
        position: 'relative'
      },
      link: {
        cursor: 'pointer',
        position: 'relative',
        padding: '0px 5px',
        display: 'block'
      },
      activeLink: {
        background: active_background_color,
      },
      toggle: {
        base: {
          position: 'relative',
          display: 'none',
          verticalAlign: 'top',
          marginLeft: '-5px',
          height: '24px',
          width: '24px'
        },
        wrapper: {
          position: 'absolute',
          top: '20%',
          left: '50%',
          margin: '-7px 0 0 -7px',
          height: '9px'
        },
        height: 9,
        width: 9,
        arrow: {
          fill: text_color,
          strokeWidth: 0
        }
      },
      header: {
          base: {
          display: 'inline-block',
          verticalAlign: 'top',
          color: text_color,
        },
        connector: {
          width: '2px',
          height: '12px',
          borderLeft: 'solid 2px black',
          borderBottom: 'solid 2px black',
          position: 'absolute',
          top: '0px',
          left: '-21px'
        },
        title: {
          lineHeight: '24px',
          verticalAlign: 'middle'
        }
      },
      subtree: {
        listStyle: 'none',
        paddingLeft: '19px'
      },
      loading: {
        color: '#E2C089'
      }
    }
  }
};

class VeloFileTree extends Component {
    static propTypes = {
        client: PropTypes.object,
        vfs_path: PropTypes.array,
        version: PropTypes.string,
        updateVFSPath: PropTypes.func,
        updateCurrentNode: PropTypes.func,
    }

    componentDidMount() {
        this.source = axios.CancelToken.source();
        this.updateTree(this.props.vfs_path);
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let prevClient = prevProps.client && prevProps.client.client_id;
        let currentClient = this.props.client && this.props.client.client_id;
        let prev_vfs_path = prevProps.vfs_path;
        let selected_filename = "";

        // If our caller did not specify a vfs path we get it from the
        // router.
        let vfs_path = this.props.vfs_path;
        if (vfs_path.length === 0) {
            // Try to extract the vfs path from the router.
            let router_vfs_path = this.props.match && this.props.match.params &&
                this.props.match.params.vfs_path;
            if (router_vfs_path) {
                vfs_path = SplitPathComponents(DecodePathInURL(router_vfs_path));

                // We need to distinguish between navigating to the
                // tree directory generally and navigating to
                // selecting a file inside the tree.  In the first
                // case the URL ends with / in the second case it does
                // not.
                // So there is a difference between /file/a/b/ and /file/a/b
                // The first form navigates the tree to the b
                // directory and leaves the file selector unset. The
                // second form navigates the tree to a directory and
                // selects the b directory.
                if (router_vfs_path.charAt(router_vfs_path.length-1) !== "/") {
                    selected_filename = vfs_path.pop();
                }
            }
        }

        // If node needs to be refreshed we reset the local cache to
        // force reloading from the server.
        if (prevProps.version !== this.props.version) {
            let node = this.state.cursor;
            node.active = true;
            node.toggled = true;
            node.inflight = false;
            node.known = false;
        }

        if (!_.isEqual(prev_vfs_path, vfs_path) ||
            prevClient !== currentClient ||
            prevProps.version !== this.props.version) {
            this.updateTree(vfs_path, function(node) {

                if(node.inflight) {
                    return;
                }

                // If we need to select a filename, search the node
                // for the file and select it.
                if(_.isEqual(node.path, vfs_path) && selected_filename) {
                    for(var i = 0; i < node.raw_data.length; i++){
                        let row = node.raw_data[i];
                        if (row.Name === selected_filename) {
                            node.selected = row.Name;
                        };
                    }
                };
            });
        }
    };

    // Update all the known data in the tree model.
    updateTree = (components, node_cb) => {
        let root_node = this.state;
        this.updateComponent(
            root_node, [], components,
            function(node, was_loaded){
                if (was_loaded) {
                    node.toggled = true;
                }
                if (node_cb) {
                    node_cb(node);
                }
            });
    }

    refreshNode = (node) => {

    }

    updateComponent = (node, prev_components, next_components, completion_cb) => {
        if (!this.props.client || !this.props.client.client_id) {
            return;
        }

        if (!completion_cb) {
            completion_cb = function(node, was_loaded){};
        }

        // This node already has valid data, consume the next
        // component and populate if needed.
        if (node.known || node.inflight) {

            // We need to go deeper.
            if (next_components && next_components.length > 0) {
                let next = next_components[0];

                next_components = next_components.slice(1);
                prev_components.push(next);

                // Search for the next file in the children.
                let children = node.children || [];
                for (var i=0; i<children.length; i++) {
                    let next_node = children[i];
                    if (next_node.name === next) {
                        this.updateComponent(next_node, prev_components,
                                             next_components, completion_cb);
                    }
                }
            }

            completion_cb(node, false);
            return;

            // This component is not known, we need to refresh from
            // the server.
        } else {
            let client_id = this.props.client.client_id;
            node.loading = true;
            node.inflight = true;

            // Cancel any in flight calls.
            this.source.cancel();
            this.source = axios.CancelToken.source();

            api.get("v1/VFSListDirectory/" + client_id, {
                vfs_path: Join(prev_components),
            }, this.source.token).then(function(response) {
                if (response.cancel) return;

                node.loading = false;
                let children = [];

                if (response && response.data && response.data.Response) {
                    // Hold on to the raw data.
                    node.raw_data = JSON.parse(response.data.Response);

                    // Extract the directory children from the raw_data
                    for (var i=0; i<node.raw_data.length; i++) {
                        let file_info = node.raw_data[i];

                        // Only show directories
                        if(file_info.Mode[0] === 'd') {
                            // Each node will contain a list of all its previous
                            // nodes in the path.
                            let path_components = [...prev_components];
                            path_components.push(file_info.Name);
                            children.push({
                                name: file_info.Name,
                                path: path_components,
                                full_path: file_info._FullPath,
                                children: [],
                            });
                        }
                    }
                }

                node.known = true;
                node.children = children;
                node.inflight = false;

                // Give our caller a chance to modify the node.
                completion_cb(node, true);

                this.props.updateCurrentNode(node);
                this.updateComponent(node, prev_components, next_components, completion_cb);
                this.setState(this.state);

            }.bind(this) , function(response) {
                node.loading = false;
                completion_cb(node, false);
            });
        };
    };

    state = {
        name: '',
        toggled: true,
        children: [],

        // The current node we are browsing.
        cursor: {},
    };

    onToggle = (node, toggled) => {
        // Deactive the current node we are on.
        const {cursor, data} = this.state;
        if (cursor) {
            cursor.active = false;
        }

        node.active = !node.active;
        node.toggled = toggled;

        this.setState(() => ({cursor: node, data: Object.assign({}, data)}));

        // Update the new vfs path
        if (node.path) {
            this.props.updateVFSPath(node.path);
        }

        let path = Join(node.path || '');
        let client_id = this.props.client.client_id || '';

        // When clicking the tree the user navigates to the directory
        // - file pane is unselected.
        this.props.history.push(EncodePathInURL("/vfs/" + client_id + path +"/"));

        node.known = false;
        this.updateComponent(node, node.path, []);
    }

    render() {
        return (
            <div className="file-tree">
              <Treebeard
                data={this.state}
                style={theme}
                onToggle={this.onToggle}
                decorators={{...decorators, Header}}
              />
            </div>
        );
    }
}

export default withRouter(VeloFileTree);
