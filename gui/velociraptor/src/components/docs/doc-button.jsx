import _ from 'lodash';

import PropTypes from 'prop-types';
import React, { Component } from 'react';

import api from '../core/api-service.jsx';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Modal from 'react-bootstrap/Modal';
import Form from 'react-bootstrap/Form';
import Col from 'react-bootstrap/Col';
import Row from 'react-bootstrap/Row';
import T from '../i8n/i8n.jsx';
import {CancelToken} from 'axios';
import Alert from 'react-bootstrap/Alert';
import Accordion from 'react-bootstrap/Accordion';
import Badge from 'react-bootstrap/Badge';
import parseHTML from '../core/sanitize.jsx';
import { cleanupHTML } from '../core/sanitize.jsx';
import hljs from 'highlight.js';
import ToolViewer from '../tools/tool-viewer.jsx';
import markdownit from 'markdown-it';
import { TablePaginationControl } from '../core/paged-table.jsx';
import InputGroup from 'react-bootstrap/InputGroup';

import "./docs.css";


class HelpDialog extends Component {
    static propTypes = {
        onClose: PropTypes.func.isRequired,
    }

    state = {
        query: "",
        hits: [],
        error: "",
        total_size: 0,
        start: 0,
        page_size: 10,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
    }

    componentWillUnmount = () => {
        this.source.cancel();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if(prevState.query != this.state.query ||
           prevState.start != this.state.start ||
           prevState.page_size != this.state.page_size) {
            this.updateSearch();
        }
    }


    updateSearch = ()=>{
        api.get("v1/SearchDocs", {
            query: this.state.query,
            start: this.state.start,
            length: this.state.page_size,
        }, this.source.token).then(response=>{
            if (response.cancel)
                return;

            let data = response && response.data;
            let items = data && data.Items;
            let total_size = data && data.total;
            this.setState({error: "",
                           total_size: parseInt(total_size || 0) ,
                           hits: items || []});
        }).catch(response=>{
            let error = response && response.response &&
                response.response.data && response.response.data.message;
            this.setState({error: error});
            return false;
        });
    }

    highlight = (text, highlights)=>{
        if(!highlights) {
            return text;
        }

        let res = [];
        let offset = 0;
        let text_len = text.length;
        for(let i=0;i<highlights.length;i++) {
            let h = highlights[i];
            let start = parseInt(h.start);
            let end = parseInt(h.end);

            if(start > text_len) {
                break;
            }

            if(start > offset) {
                res.push(text.slice(offset, start));
            }

            res.push("<span class='highlight'>" + text.slice(start, end) + "</span>");
            offset = end;
        }

        if(offset < text_len) {
            res.push(text.slice(offset, text_len));
        }

        return res.join("");
    }

    searchTag = tag=>{
        this.setState({query: "tags:" + tag + " " + this.state.query});
    }


    renderTags = tags=>{
        return _.map(tags, (x, idx)=>{
            return <Badge
                     onClick={()=>this.searchTag(x)}
                     className="tag"
                     key={idx}>{x}</Badge>;
        });
    }

    sanitize = text=>{
        return parseHTML(text);
    }

    render() {
        const md = markdownit({
            html: true,
            linkify: false,
            highlight: function (str, lang) {
                if (lang && hljs.getLanguage(lang)) {
                    try {
                        return hljs.highlight(str, { language: lang }).value;
                    } catch (__) {}
                }

                return ''; // use external default escaping
            }
        });

        return (
            <Modal show={true}
                   dialogClassName="modal-90w"
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Search Documentation")}</Modal.Title>
              </Modal.Header>
              <Modal.Body className="search-hit">
                <Form>
                  <InputGroup>
                      <Form.Control
                        spellCheck="false"
                        value={this.state.query}
                        onChange={e=>this.setState({query: e.target.value})}
                        as="input"
                        placeholder={T("Search Docs")} />
                      <TablePaginationControl
                        total_size={this.state.total_size}
                        start_row={this.state.start}
                        page_size={this.state.page_size}
                        current_page={this.state.start_row / this.state.page_size}
                        onRowChange={row_offset=>{
                            this.setState({start: row_offset});
                        }}
                        onPageSizeChange={size=>this.setState({page_size: size})}
                      />
                  </InputGroup>
                </Form>
                <Accordion>
                  {_.map(this.state.hits, (v, idx)=>{
                      let full_text = this.highlight(v.full_text, v.highlights);
                      full_text = md.render(full_text || "");
                      return <Accordion.Item eventKey={idx} key={idx}>
                               <Accordion.Header>
                                 <Row className="hit-row">
                                   <Col sm="2">{this.renderTags(v.tags)}</Col>
                                   <Col sm="10">
                                     <a target="_blank" href={v.link}>
                                       {v.title}
                                     </a>
                                   </Col>
                                 </Row>
                               </Accordion.Header>
                               <Accordion.Body>
                                 {this.sanitize(full_text)}
                               </Accordion.Body>
                             </Accordion.Item>;
                  })}
                </Accordion>
              </Modal.Body>
              <Modal.Footer>
                { this.state.error &&
                  <Alert className="doc-error-bar"
                         variant="warning">{this.state.error}</Alert>
                }
                <ToolViewer key="3" name="DocsIndex" />
                <Button variant="secondary"
                        onClick={()=>this.props.onClose()}>
                  {T("Close")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

export default class DocButton extends Component {
    state = {
        show_dialog: false,
    }

    render() {
        return (
            <>
              <Button
                variant="primary"
                className="help-button"
                onClick={()=>this.setState({show_dialog: true})}
              >
                <FontAwesomeIcon icon="circle-question" />
              </Button>
              { this.state.show_dialog &&
                <HelpDialog
                  onClose={()=>this.setState({show_dialog: false})}
                /> }
            </>
        );
    }
}
