import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';
import api from '../core/api-service.js';
import Pagination from '../bootstrap/pagination/index.js';
import Spinner from '../utils/spinner.js';
import utils from './utils.js';
import VeloAce from '../core/ace.js';
import axios from 'axios';
import "./file-hex-view.css";
import T from '../i8n/i8n.js';

const pagesize = 100 * 1024;

export default class FileTextView extends React.Component {
    static propTypes = {
        node: PropTypes.object,
        client: PropTypes.object,
    };

    state = {
        page: 0,
        rawdata: "",
        loading: false,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.fetchText_(0);
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        // Update the view when
        // 1. Selected node changes (file list was selected).
        // 2. VFS path changes (tree navigated away).
        // 3. node version changes (file was refreshed).
        // 4. page is changed
        if (prevProps.node.selected !== this.props.node.selected ||
            !_.isEqual(prevProps.node.path, this.props.node.path) ||
            prevProps.node.version !== this.props.node.version ||
            prevState.page !== this.state.page) {
            this.fetchText_(this.state.page);
        };
    }

    fetchText_ = (page) => {
        let selectedRow = utils.getSelectedRow(this.props.node);
        let client_id = this.props.client && this.props.client.client_id;
        if (!client_id) {
            return;
        }

        let name = selectedRow && selectedRow.Name;
        if (!name) {
            return;
        }

        let fileComponents = this.props.node.path.slice();
        if (_.isEmpty(fileComponents)) {
            return;
        }
        fileComponents.push(name);

        let vfs_path = selectedRow.Download && selectedRow.Download.vfs_path;

        var url = 'v1/DownloadVFSFile';
        var params = {
            offset: page * pagesize,
            length: pagesize,
            components: fileComponents,
            client_id: client_id,
            vfs_path: vfs_path,
        };

        this.setState({loading: true});
        api.get_blob(url, params, this.source.token).then(buffer=>{
            const view = new Uint8Array(buffer);
            this.parseFileContentToTextRepresentation_(view, page);
        }, ()=>{
            this.setState({hexDataRows: [], loading: false, page: page});
        });
    };

    parseFileContentToTextRepresentation_ = (intArray, page) => {
        let rawdata = "";
        let line_length = 0;
        for (var i = 0; i < intArray.length; i++) {
            if(intArray[i] > 0x20 && intArray[i]<0x7f) {
                rawdata += String.fromCharCode(intArray[i]);
            } else {
                rawdata += ".";
            }
            line_length += 1;
            if (intArray[i] === 0xa) {
                line_length = 0;
                rawdata += "\n";
            }
            if (line_length > 80) {
                rawdata += "\n";
                line_length = 0;
            }
        };
        this.setState({rawdata: rawdata, loading: false, page: page});
    };


    render() {
        let selectedRow = utils.getSelectedRow(this.props.node);
        let mtime = selectedRow && selectedRow.Download && selectedRow.Download.mtime;
        if (!mtime) {
            return <h5 className="no-content">{T("File has no data, please collect file first.")}</h5>;
        }
        var total_size = selectedRow.Size || 0;
        let pageCount = Math.ceil(total_size / pagesize);
        let paginationConfig = {
            totalPages: pageCount,
            currentPage: this.state.page + 1,
            showMax: 5,
            size: "sm",
            threeDots: true,
            center: true,
            prevNext: true,
            shadow: true,
            onClick: (page, e) => {
                this.setState({page: page - 1});
                e.preventDefault();
                e.stopPropagation();
            },
        };

        return (
            <div>
              <Spinner loading={this.state.loading}/>
              <div className="file-hex-view">
                <Pagination {...paginationConfig} />
                <VeloAce
                  text={this.state.rawdata}
                  mode="text"
                  options={{readOnly: true}}
                />
              </div>
            </div>
        );
    }
};
