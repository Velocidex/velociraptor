import './browse-vfs.css';
import _ from 'lodash';

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import api from '../core/api-service.jsx';

import SplitPane from 'react-split-pane';
import {CancelToken} from 'axios';

import VeloFileTree from './file-tree.jsx';
import VeloFileList from './file-list.jsx';
import VeloFileStats from './file-stats.jsx';

const POLL_TIME = 5000;


class VFSViewer extends Component {
    static propTypes = {
        client: PropTypes.object,
        updateCurrentNode: PropTypes.func.isRequired,
        node: PropTypes.object,
        vfs_path: PropTypes.array,
    }

    state = {
        topPaneSize: undefined,
        collapsed: false,
        vfs_path: [],
        current_node: {},
        current_selected_row: {},

        version: "",
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.interval = setInterval(this.statDirectory, POLL_TIME);
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        if (this.interval) {
            clearInterval(this.interval);
        }
    }

    statDirectory = ()=>{
        let path = this.props.node.path || [];
        let node = this.state.current_node;

        api.get("v1/VFSStatDirectory", {
            client_id: this.props.client.client_id,
            vfs_components: path,
        }, this.source.token).then((response) => {
            if (response.data.cancel) {
                return;
            }
            let ts = response.data.timestamp || 0;
            let download_version = response.data.download_version || 0;
            let version = download_version + " " + ts;
            node.start_idx = response.data.start_idx;
            node.end_idx = response.data.end_idx;
            node.flow_id = response.data.flow_id;

            this.setState({version: version});
        });
    }

    bumpVersion = ()=>{
        this.statDirectory();
    }

    collapse = () => {
        if (!this.state.collapsed) {
            this.setState({topPaneSize: "100%", collapsed: true});
        } else {
            this.setState({topPaneSize: "50%", collapsed: false});
        }
    }

    updateVFSPath = (vfs_path) => {
        if (!_.isEqual(vfs_path, this.state.vfs_path)) {
            this.setState({vfs_path: vfs_path, version: ""});
        }
    };

    updateCurrentNode = (node) => {
        if (!_.isEqual(node, this.state.current_node)) {
            this.setState({current_node: node, version: ""});
            this.props.updateCurrentNode(node);
        }
    }

    updateCurrentSelectedRow = (row) => {
        this.setState({current_selected_row: Object.assign({_id: 1}, row)});
    }

    render() {
        return (
            <>
            <SplitPane split="vertical"
                       defaultSize="20%">
              <VeloFileTree client={this.props.client}
                            version={this.props.node && this.props.node.version}
                            className="file-tree"
                            updateVFSPath={this.updateVFSPath}
                            updateCurrentNode={this.updateCurrentNode}
                            vfs_path={this.state.vfs_path} />
              <SplitPane split="horizontal"
                         defaultSize="50%"
                         size={this.state.topPaneSize}
                         onResizerDoubleClick={this.collapse}>
                <VeloFileList
                  client={this.props.client}
                  collapseToggle={this.collapse}
                  updateCurrentNode={this.updateCurrentNode}
                  updateCurrentSelectedRow={this.updateCurrentSelectedRow}
                  version={this.state.version}
                  bumpVersion={this.bumpVersion}
                  node={this.state.current_node} />
                <VeloFileStats
                  client={this.props.client}
                  version={this.state.version}
                  bumpVersion={this.bumpVersion}
                  updateCurrentNode={this.updateCurrentNode}
                  selectedRow={this.state.current_selected_row}
                  node={this.state.current_node} />
              </SplitPane>
            </SplitPane>
            </>
        );
    }
}

export default VFSViewer;
