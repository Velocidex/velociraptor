import React, { Component } from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';

import {Treebeard} from 'react-treebeard';

import {Join, SplitPathComponents} from '../utils/paths.js';

import api from '../core/api-service.js';
import { withRouter }  from "react-router-dom";

const text_color = "#8f8f8f";
const background_color = '#f5f5f5';
const active_background_color = "#dee0ff";

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
        marginTop: '-24px',
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
          display: 'inline-block',
          verticalAlign: 'top',
          marginLeft: '-5px',
          height: '24px',
          width: '24px'
        },
        wrapper: {
          position: 'absolute',
          top: '50%',
          left: '50%',
          margin: '-7px 0 0 -7px',
          height: '14px'
        },
        height: 14,
        width: 14,
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
        updateVFSPath: PropTypes.func,
    }

    componentDidMount() {
        this.updateTree();
    }

    componentDidUpdate(prevProps) {
        let prevClient = prevProps.client && prevProps.client.client_id;
        let currentClient = this.props.client && this.props.client.client_id;
        let prev_vfs_path = prevProps.vfs_path;

        if (!_.isEqual(prev_vfs_path, this.props.vfs_path) ||
            prevClient != currentClient ) {
            this.updateTree();
        }
    };

    // Update all the known data in the tree model.
    updateTree = () => {
        let components = this.props.vfs_path;
        let root_node = this.state;
        this.updateComponent(root_node, [], components);
    }

    updateComponent = (node, prev_components, next_components) => {
        if (!this.props.client || !this.props.client.client_id) {
            return;
        }

        // This node already has valid data, consume the next
        // component and populate if needed.
        if (node.known) {

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
                        this.updateComponent(next_node, prev_components, next_components);
                    }
                }
            }

            // This component is not known, we need to refresh from
            // the server.
        } else {
            let client_id = this.props.client.client_id;
            node.loading = true;
            api.get("api/v1/VFSListDirectory/" + client_id, {
                vfs_path: Join(prev_components),
            }).then(function(response) {
                node.loading = false;

                if (!response || !response.data || !response.data.Response) {
                    return;
                }

                // Hold on to the raw data.
                node.raw_data = JSON.parse(response.data.Response);

                // Extract the directory children from the raw_data
                let children = [];
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
                            children: [],
                        });
                    }
                }
                node.known = true;
                node.children = children;
                node.toggled = true;

                this.updateComponent(node, prev_components, next_components);
                this.setState(this.state);
            }.bind(this), function(response) {
                node.loading = false;
            });
        };
    }

    state = {
        name: '',
        toggled: true,
        children: [],

        // The current node we are browsing.
        cursor: {},
    };

    onToggle = (node, toggled) => {
        const {cursor, data} = this.state;
        if (cursor) {
            cursor.active = false;
        }
        node.active = !node.active;
        if (node.children) {
            node.toggled = toggled;
        }
        this.setState(() => ({cursor: node, data: Object.assign({}, data)}));

        // Update the new vfs path
        this.props.updateVFSPath(node.path);

        let path = Join(node.path || '');
        let client_id = this.props.client.client_id || '';
        this.props.history.push("/vfs/" + client_id + path);
    }

    render() {
        return (
            <div className="file-tree">
              <Treebeard
                data={this.state}
                style={theme}
                onToggle={this.onToggle}
              />
            </div>
        );
    }
}

export default withRouter(VeloFileTree);
