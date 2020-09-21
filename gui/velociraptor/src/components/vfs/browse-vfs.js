import './browse-vfs.css';

import React, { Component, useState } from 'react';
import PropTypes from 'prop-types';

import SplitPane from 'react-split-pane';
import Pane from 'react-split-pane';

import VeloFileTree from './file-tree.js';

const resizerStyle = {
//    width: "25px",
};

class VFSViewer extends Component {
    static propTypes = {
        client: PropTypes.object,
        vfs_path: PropTypes.array,
        updateVFSPath: PropTypes.func,
    }

    state = {
        topPaneSize: undefined,
        collapsed: false,
    }

    collapse = () => {
        if (this.state.collapsed) {
            this.setState({topPaneSize: undefined, collapsed: false});
        } else {
            this.setState({topPaneSize: "10px", collapsed: true});
        }
    }

    render() {
        return (
            <>
            <SplitPane split="vertical"
                       defaultSize="20%"
                       resizerStyle={resizerStyle}>
              <VeloFileTree client={this.props.client}
                            className="file-tree"
                            updateVFSPath={this.props.updateVFSPath}
                            vfs_path={this.props.vfs_path} />
              <SplitPane split="horizontal"
                         defaultSize="50%"
                         size={this.state.topPaneSize}
                         onResizerDoubleClick={this.collapse}
                         resizerStyle={resizerStyle}>
                <div>Top world {this.props.vfs_path}</div>
                <div>Bottom world</div>
              </SplitPane>
            </SplitPane>
            </>
        );
    };
}

export default VFSViewer;
