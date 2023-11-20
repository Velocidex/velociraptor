import "./preview_uploads.css";

import _ from 'lodash';

import PropTypes from 'prop-types';
import React, { Component } from 'react';
import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';
import Button from 'react-bootstrap/Button';
import qs from 'qs';
import Modal from 'react-bootstrap/Modal';
import Container from  'react-bootstrap/Container';
import HexView from '../utils/hex.jsx';
import SearchHex from './search.jsx';
import Spinner from '../utils/spinner.jsx';
import HexPaginationControl from './pagination.jsx';
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
        this.source = CancelToken.source();
        this.fetchPage_(0);
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (!_.isEqual(prevProps.params, this.props.params) ||
            !_.isEqual(prevState.page, this.state.page) ||
            !_.isEqual(prevState.columns, this.state.columns)) {
            this.fetchPage_(this.state.page);
        };
    }

    state = {
        // The offset in the file where this screen views
        base_offset: 0,
        page: 0,
        rows: 25,
        columns: 0x10,
        view: undefined,
        loading: true,
        textview_only: false,
        highlights: {},
        highlight_version: 0,
        version: 0,
    }

    fetchPage_ = (page, ondone) => {
        let params = Object.assign({}, this.props.params);

        this.source.cancel();
        this.source = CancelToken.source();

        // read a bit more than we need to so the text view looks a
        // bit more full.
        params.length = this.state.rows * this.state.columns * 2;
        params.offset = page * this.state.rows * this.state.columns;

        api.get_blob(this.props.url, params, this.source.token).then(buffer=>{
            const view = new Uint8Array(buffer);
            this.setState({
                base_offset: params.offset,
                view: view,
                version: this.state.version+1,
                rawdata: this.parseFileContentToTextRepresentation_(view),
                loading: false});
            if(ondone) {ondone();};
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

        return (
            <Container className="file-hex-view">
              <Spinner loading={this.state.loading}/>
              <Row>
                <Col sm="1">
                  <Button variant="secondary"
                          className="page-link hex-goto"
                          onClick={()=>this.setState({
                              textview_only: !this.state.textview_only,
                          })}>
                    <FontAwesomeIcon icon="text-height"/>
                  </Button>
                </Col>
                <Col sm="3">
                  <HexPaginationControl
                    page_size={chunkSize}
                    total_size={total_size}
                    total_pages={pageCount}
                    current_page={this.state.page}
                    set_highlights={(name, hits)=>{
                        let h = this.state.highlights;
                        h[name] = hits;
                        this.setState({
                            highlights: h,
                            highlight_version: this.state.highlight_version+1});
                    }}
                    onPageChange={page=>{
                        this.fetchPage_(page, ()=>{
                            this.setState({
                                base_offset: page * chunkSize,
                                page: page,
                            });
                        });
                    }}
                  />
                </Col>
                <Col sm="8" className="hex-goto">
                  <SearchHex
                    base_offset={this.state.base_offset}
                    vfs_components={this.props.params &&
                                    this.props.params.fs_components}
                    page_size={chunkSize}
                    current_page={this.state.page}
                    byte_array={this.state.view}
                    version={this.state.version}
                    onPageChange={page=>{
                        this.fetchPage_(page, ()=>{
                            this.setState({
                                base_offset: page * chunkSize,
                                page: page,
                            });
                        });
                    }}
                    set_highlights={(name, hits)=>{
                        let h = this.state.highlights;
                        h[name] = hits;
                        this.setState({
                            highlights: h,
                            highlight_version: this.state.highlight_version+1});
                    }}/>
                </Col>
              </Row>
              <Row>
                { this.state.textview_only ?
                  <Col sm="12">
                    <div className="panel textdump">
                      {this.state.rawdata}
                    </div>
                  </Col>
                  :
                  <>
                    <Col sm="12" className="hexdump-pane">
                      <div className="panel hexdump">
                        <HexView
                          // Highlights on top of the data.
                          highlights={this.state.highlights}
                          highlight_version={this.state.highlight_version}
                          base_offset={this.state.base_offset}
                          height={this.state.rows}
                          rows={this.state.rows}
                          setColumns={v=>this.setState({columns: v})}
                          columns={this.state.columns}

                          // The data that will be rendered
                          byte_array={this.state.view}
                          version={this.state.version} />
                      </div>
                    </Col>
                  </>
                }
              </Row>
            </Container>
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
                      <div className="preview-json">
                        <VeloValueRenderer value={this.props.upload}/>
                      </div>}
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
        error: false,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.fetchPreview_();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (!_.isEqual(prevProps.upload, this.props.upload)) {
            this.fetchPreview_();
        };

        return _.isEqual(this.state.view, prevState.view) &&
            _.isEqual(this.state.error, prevState.error) ;
    }

    isValidEnv = env=>{
        if(env.hunt_id) return true;
        if(env.notebook_cell_id) return true;
        if(env.client_id && env.flow_id) return true;
        if(env.client_id && env.vfs_components) return true;
        return false;
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
        if (!this.isValidEnv(env)) {
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
            padding: this.props.upload.Padding,
            fs_components: normalizeComponentList(
                components, client_id, flow_id, accessor),
            client_id: client_id,
            org_id: window.globals.OrgId || "root",
        };
        let url = 'v1/DownloadVFSFile';

        this.setState({url: url, params: params,
                       error: false, loading: true});

        api.get_blob(url, params, this.source.token).then(buffer=>{
            if(buffer.error) {
                this.setState({error: true});

            } else {
                const view = new Uint8Array(buffer);
                this.setState({view: view, error: false});
            }
        });
    };

    uintToString = (uintArray) => {
        return String.fromCharCode.apply(null, uintArray);
    }

    render() {
        if (this.state.error) {
            return <FontAwesomeIcon icon="circle-exclamation" />;
        }

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
                org_id: window.globals.OrgId || "root",

                // Only view first 1mb
                length: 1000000,
            };
            params["fs_components[]"] = this.state.params.fs_components;
            let url = api.base_path + "/api/" + this.state.url + "?" +
                 qs.stringify(params, {indices: false});
            string_data = <img className="preview-thumbnail"
                               src={url} alt="preview upload" />;
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

    switch (components[0]) {
    case "clients":
    case "notebooks":
    case "hunts":
        // It is a filestore path already
        return components;

    case "uploads":
        // It is given relative to the client's flow.
        return ["clients", client_id,
                "collections", flow_id].concat(components);
    }

    return ["clients", client_id, "collections", flow_id,
            "uploads", accessor].concat(components);
};
