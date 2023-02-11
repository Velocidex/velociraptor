import "./preview_uploads.css";

import _ from 'lodash';

import PropTypes from 'prop-types';
import React, { Component } from 'react';
import axios from 'axios';
import api from '../core/api-service.jsx';
import { HexViewDialog } from '../utils/hex.jsx';
import Button from 'react-bootstrap/Button';
import qs from 'qs';
import Modal from 'react-bootstrap/Modal';
import HexView from '../utils/hex.jsx';
import Spinner from '../utils/spinner.jsx';
import Pagination from '../bootstrap/pagination/index.jsx';
import Tab from 'react-bootstrap/Tab';
import Tabs from 'react-bootstrap/Tabs';
import T from '../i8n/i8n.jsx';
import VeloValueRenderer from '../utils/value.jsx';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

// https://en.wikipedia.org/wiki/List_of_file_signatures
const patterns = [
    ["image/png", [0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A]],
    ["image/jpg", [0xFF, 0xD8, 0xFF, 0xDB]],
    ["image/jpg", [0xFF, 0xD8, 0xFF, 0xE0, "?", "?", 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01]],
    ["image/jpg", [0xFF, 0xD8, 0xFF, 0xE1, "?", "?", 0x45, 0x78, 0x69, 0x66, 0x00, 0x00]],
];

function checkMime(buffer) {
    for(let i=0; i<patterns.length; i++) {
        let pattern = patterns[i][1];
        let matched = true;
        for (let j=0; j<pattern.length; j++) {
            // Check the next pattern
            if (j > buffer.length) {
                matched = false;
                break;
            }
            // Skip this check
            if (pattern[j] === "?") {
                continue ;
            }

            if (pattern[j] !== buffer[j]) {
                matched = false;
                break;
            }
        }
        if (matched) {
            return patterns[i][0];
        }
    }
    return "";
}

class HexViewTab  extends React.PureComponent {
    static propTypes = {
        params:  PropTypes.object,
        url:     PropTypes.string,
        size:    PropTypes.number,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.fetchPage_();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (!_.isEqual(prevProps.params, this.props.params) ||
            !_.isEqual(prevState.page, this.state.page)) {
            this.fetchPage_();
        };
    }

    state = {
        page: 0,
        rows: 25,
        columns: 0x10,
        view: undefined,
        loading: true,
        textview_only: false,
    }

    fetchPage_ = () => {
        let params = Object.assign({}, this.props.params);

        this.source.cancel();
        this.source = axios.CancelToken.source();

        // read a bit more than we need to so the text view looks a
        // bit more full.
        params.length = this.state.rows * this.state.columns * 2;
        params.offset = this.state.page * this.state.rows * this.state.columns;

        api.get_blob(this.props.url, params, this.source.token).then(buffer=>{
            const view = new Uint8Array(buffer);
            this.setState({
                view: view,
                rawdata: this.parseFileContentToTextRepresentation_(view),
                loading: false});
        });
        this.setState({loading: true});
    }

    parseFileContentToTextRepresentation_ = intArray=>{
        let rawdata = "";
        for (var i = 0; i < intArray.length; i++) {
            let c = intArray[i];
            // Skip nulls to compress utf16
            if (c == 0) {
                continue;
            }

            if(c >= 0x20 && c<0x7f ||
               c === 10 || c === 13 || c === 9) {
                rawdata += String.fromCharCode(intArray[i]);
            } else {
                rawdata += ".";
            }
        };
        return rawdata;
    };


    render() {
        var chunkSize = this.state.rows * this.state.columns;
        let total_size = this.props.size || 0;
        let pageCount = Math.ceil(total_size / chunkSize);
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
                this.fetchPage_(page - 1);
                e.preventDefault();
                e.stopPropagation();
            },
        };

        return (
            <div className="file-hex-view">
              <Spinner loading={this.state.loading}/>
              { <Pagination {...paginationConfig}/> }
              <Row>
                { this.state.textview_only ?
                  <Col sm="12">
                    <Button variant="secondary"
                            className="activate-button"
                            onClick={()=>this.setState({
                                textview_only: false
                            })}>
                      <FontAwesomeIcon icon="compress"/>
                    </Button>
                    <div className="panel textdump">
                      <pre>{this.state.rawdata}</pre>
                    </div>
                  </Col>
                  :
                  <>
                    <Col sm="8">
                      <div className="panel hexdump">
                        <HexView
                          height={this.state.rows}
                          rows={this.state.rows}
                          columns={this.state.columns}
                          byte_array={this.state.view} />
                      </div>
                    </Col>

                    <Col sm="4">
                      <Button variant="secondary"
                              className="activate-button"
                              onClick={()=>this.setState({
                                  textview_only: true
                              })}>
                        <FontAwesomeIcon icon="expand"/>
                      </Button>
                      <div className="panel textdump">
                        <pre>{this.state.rawdata}</pre>
                      </div>
                    </Col>
                  </>
                }
              </Row>
            </div>
        );
    }
}

class InspectDialog extends React.PureComponent {
    static propTypes = {
        params:  PropTypes.object,
        url:     PropTypes.string,
        size:    PropTypes.number,
        upload: PropTypes.any,
        onClose: PropTypes.func.isRequired,
    }

    state = {
        tab: "overview",
    }

    render() {
        return (
            <Modal show={true}
                   dialogClassName="modal-90w"
                   enforceFocus={true}
                   className="full-height"
                   scrollable={true}
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>Inspect File</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <Tabs activeKey={this.state.tab}
                      onSelect={tab=>this.setState({tab: tab})}>
                  <Tab eventKey="overview" title={T("Overview")}>
                    { this.state.tab === "overview" &&
                      <HexViewTab params={this.props.params}
                                  url={this.props.url}
                                  size={this.props.size}
                      />}
                  </Tab>
                  <Tab eventKey="details" title={T("Details")}>
                    { this.state.tab === "details" &&
                      <VeloValueRenderer value={this.props.upload}/>}
                  </Tab>
                </Tabs>
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  Close
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}


export default class PreviewUpload extends Component {
    static propTypes = {
        env: PropTypes.object,
        upload: PropTypes.any,
    }

    state = {
        page: 0,
        columns: 0x10,
        hexDataRows: [],
        view: undefined,
        showDialog: false,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.fetchPreview_();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (!_.isEqual(prevProps.upload, this.props.upload)) {
            this.fetchPreview_();
        };
    }

    fetchPreview_ = () => {
        let accessor = this.props.upload.Accessor || "auto";
        let components = this.props.upload.Components;
        if (_.isUndefined(components)) {
            // No components available - do our best
            let path = (this.props.upload.Path || "");
            if (path.includes("\\")) {
                components = path.split("\\");
            } else {
                components = path.split("/");
            }
        }
        let env = this.props.env || {};
        let client_id = env.client_id;
        let flow_id = env.flow_id;
        if(!client_id || !flow_id) {
            return;
        };

        components = components.filter(x=>x);

        if (_.isEmpty(components)) {
            return;
        }

        // First get a small header to figure out what to do.
        let params = {
            offset: 0,
            length: 100,
            fs_components: normalizeComponentList(
                components, client_id, flow_id, accessor),
            client_id: client_id,
            org_id: window.globals.OrgId || "root",
        };
        let url = 'v1/DownloadVFSFile';

        this.setState({url: url, params: params, loading: true});

        api.get_blob(url, params, this.source.token).then(buffer=>{
            const view = new Uint8Array(buffer);
            this.setState({view: view});
        });
    };

    uintToString = (uintArray) => {
        return String.fromCharCode.apply(null, uintArray);
    }

    render() {
        if (_.isString(this.props.upload)) {
            return <>{this.props.upload}</>;
        }
        if (_.isEmpty(this.props.upload) ||
            !_.isObject(this.props.upload) ||
            _.isEmpty(this.state.view) ||
            !this.props.upload.Size ) {
            return <></>;
        }

        let string_data = this.uintToString(this.state.view);
        if (string_data.length > 20) {
            string_data = string_data.substring(0, 20) + "...";
        }

        // Match the data in case it is an image
        if (checkMime(this.state.view)) {
            let params = {
                client_id: this.props.env.client_id,

                // Only view first 1mb
                length: 1000000,
            };
            params["fs_components[]"] = this.state.params.fs_components;
            let url = api.base_path + "/api/" + this.state.url + "?" +
                 qs.stringify(params, {indices: false});
            string_data = <img className="preview-thumbnail" src={url}/>;
        }

        return (
            <>
              { this.state.showDialog && this.state.params &&
                <InspectDialog params={this.state.params}
                               url={this.state.url}
                               size={this.props.upload.StoredSize ||
                                     this.props.upload.Size || 0}
                               upload={this.props.upload}
                               onClose={()=>this.setState({showDialog: false})}
                /> }
              <Button className="hex-popup client-link"
                      size="sm"
                      onClick={()=>this.setState({showDialog: true})}
                      variant="outline-info">
                {string_data}
              </Button>
            </>
        );
    }
}

const normalizeComponentList = (components, client_id, flow_id, accessor)=>{
    if (!components) {
        return [accessor];
    }

    // It is a filestore path already
    if (components[0] === "clients") {
        return components;
    }

    if (components[0] === "uploads") {
    return ["clients", client_id,
            "collections", flow_id].concat(components);
    }

    return ["clients", client_id, "collections", flow_id,
            "uploads", accessor].concat(components);
};
