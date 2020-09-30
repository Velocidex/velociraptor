import './browse-vfs.css';

import React, { Component } from 'react';
import PropTypes from 'prop-types';

import SplitPane from 'react-split-pane';

import VeloFileTree from './file-tree.js';
import VeloFileList from './file-list.js';
import VeloFileDetails from './file-details.js';

const resizerStyle = {
//    width: "25px",
};

class VFSViewer extends Component {
    static propTypes = {
        client: PropTypes.object,
    }

    state = {
        topPaneSize: undefined,
        collapsed: false,
        vfs_path: [],
        selected_row: {},
        current_node: {},
    }

    collapse = () => {
        if (this.state.collapsed) {
            this.setState({topPaneSize: undefined, collapsed: false});
        } else {
            this.setState({topPaneSize: "10px", collapsed: true});
        }
    }

    updateVFSPath = (vfs_path) => {
        this.setState({vfs_path: vfs_path});
    };

    updateCurrentNode = (node) => {
        this.setState({current_node: node});
    }

    render() {
        return (
            <>
            <SplitPane split="vertical"
                       defaultSize="20%"
                       resizerStyle={resizerStyle}>
              <VeloFileTree client={this.props.client}
                            className="file-tree"
                            updateVFSPath={this.updateVFSPath}
                            updateCurrentNode={this.updateCurrentNode}
                            vfs_path={this.state.vfs_path} />
              <SplitPane split="horizontal"
                         defaultSize="50%"
                         size={this.state.topPaneSize}
                         onResizerDoubleClick={this.collapse}
                         resizerStyle={resizerStyle}>
                <VeloFileList
                  client={this.props.client}
                  updateCurrentNode={this.updateCurrentNode}
                  node={this.state.current_node} />
                <VeloFileDetails
                  client={this.props.client}
                   node={this.state.current_node} />
              </SplitPane>
            </SplitPane>
            </>
        );
    };
}

export default VFSViewer;
