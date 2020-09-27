import './browse-vfs.css';

import React, { Component, useState } from 'react';
import PropTypes from 'prop-types';

import SplitPane from 'react-split-pane';
import Pane from 'react-split-pane';

import VeloFileTree from './file-tree.js';
import VeloFileList from './file-list.js';
import VeloFileDetails from './file-details.js';

const resizerStyle = {
//    width: "25px",
};

class VFSViewer extends Component {
    static propTypes = {
        client: PropTypes.object,
        vfs_path: PropTypes.array,
        selectedRow: PropTypes.object,
        node: PropTypes.object,
        setSelectedRow: PropTypes.func,
        updateCurrentNode: PropTypes.func,
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
                            setSelectedRow={this.props.setSelectedRow}
                            updateVFSPath={this.props.updateVFSPath}
                            updateCurrentNode={this.props.updateCurrentNode}
                            vfs_path={this.props.vfs_path} />
              <SplitPane split="horizontal"
                         defaultSize="50%"
                         size={this.state.topPaneSize}
                         onResizerDoubleClick={this.collapse}
                         resizerStyle={resizerStyle}>
                <VeloFileList
                  client={this.props.client}
                  setSelectedRow={this.props.setSelectedRow}
                  node={this.props.node} />
                <VeloFileDetails
                  selectedRow={this.props.selectedRow} />
              </SplitPane>
            </SplitPane>
            </>
        );
    };
}

export default VFSViewer;
