import "./hex.css";

import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Modal from 'react-bootstrap/Modal';

class HexViewDialog extends React.PureComponent {
    static propTypes = {
        data: PropTypes.string,
        onClose: PropTypes.func.isRequired,
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
                <Modal.Title>Hex View data</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <div className="hex-dialog-body">
                  <HexView data={this.props.data}  height={50}/>
                </div>
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




export class HexViewPopup extends React.Component {
    static propTypes = {
        data: PropTypes.string,
    };

    state = {
        showDialog: false,
    }

    render() {
        let string_data = _.toString(this.props.data);
        if (string_data.length > 10) {
            string_data = string_data.substring(0, 10) + "...";
        }
        string_data = string_data.replace(/[^\x20-\x7E ]/g, '');
        return (
            <>
              { this.state.showDialog &&
                <HexViewDialog data={this.props.data}
                               onClose={()=>this.setState({showDialog: false})}
                /> }
              <Button className="hex-popup client-link"
                      size="sm"
                      onClick={()=>this.setState({showDialog: true})}
                      variant="outline-info">
                <FontAwesomeIcon icon="external-link-alt"/>
                {string_data}
              </Button>
            </>
        );
    }
};



// A hex viewer suitable for small amountfs of text - No paging.
export default class HexView extends React.Component {
    static propTypes = {
        data: PropTypes.string,
        height: PropTypes.number,
        max_height: PropTypes.number,
        columns: PropTypes.number,
    };

    state = {
        hexDataRows: [],
        parsed_data: "",
        expanded: false,
    }

    componentDidMount = () => {
        this.parseFileContentToHexRepresentation_(this.props.data);
        this.setState({parsed_data: this.props.data});
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        return this.props.data !== this.state.parsed_data;
    }

    parseFileContentToHexRepresentation_ = (fileContent) => {
        if (!fileContent) {
            fileContent = "";
        }

        // The absolute maximum height we will render.
        let max_height = this.props.max_height || 1000;
        let columns = this.props.columns || 16;
        let hexDataRows = [];
        for(var i = 0; i < max_height; i++){
            let offset = 0;
            var rowOffset = offset + (i * columns);
            var data = fileContent.substr(i * columns, columns);
            var data_row = [];
            for (var j = 0; j < data.length; j++) {
                var char = data.charCodeAt(j).toString(16);
                data_row.push(('0' + char).substr(-2)); // add leading zero if necessary
            };

            if (data_row.length === 0) {
                break;
            };

            let safe_data = data.replace(/[^\x20-\x7f]/g, '.');
            safe_data = safe_data.split(" ");
            hexDataRows.push({
                offset: rowOffset,
                data_row: data_row,
                data: data,
                safe_data: safe_data,
            });

        }

        this.setState({hexDataRows: hexDataRows, loading: false});
    };


    render() {
        let height = this.props.height || 5;
        let more = this.state.hexDataRows.length > height;
        let hexArea =
            <table className="hex-area">
              <tbody>
                { _.map(this.state.hexDataRows, (row, idx)=>{
                    if (idx >= height && !this.state.expanded) {
                        return <></>;
                    }
                    return <tr key={idx}>
                             <td>
                               { _.map(row.data_row, (x, idx)=>{
                                   return <span key={idx}>{ x }&nbsp;</span>;
                               })}
                             </td>
                           </tr>; })
                }
              </tbody>
            </table>;

        let contextArea =
            <table className="content-area">
              <tbody>
                { _.map(this.state.hexDataRows, (row, idx)=>{
                    if (idx >= height && !this.state.expanded) {
                        return <></>;
                    }
                    return <tr key={idx}>
                             <td className="data">
                               { _.map(row.safe_data, (x, idx)=>{
                                   return <span key={idx}>{ x }&nbsp;</span>;
                               })}
                             </td>
                           </tr>;
                })}
              </tbody>
            </table>;

        return (
            <div className="panel hexdump">
              <div className="monospace">
                <table>
                  <thead>
                    <tr>
                      <th className="offset-area">
                        Offset
                      </th>
                      <th className="padding-area">
                        00 01 02 03 04 05 06 07 08 09 0a 0b 0c 0d 0e 0f
                      </th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      <td>
                        <table className="offset-area">
                          <tbody>
                            { _.map(this.state.hexDataRows, (row, idx)=>{
                                if (idx >= height && !this.state.expanded) {
                                    return <></>;
                                }
                                return <tr key={idx}>
                                         <td className="offset">
                                           { row.offset }
                                         </td>
                                       </tr>; })}
                          </tbody>
                        </table>
                      </td>
                      <td>
                        { hexArea }
                      </td>
                      <td>
                        { contextArea }
                      </td>
                    </tr>
                    { more && (this.state.expanded ?
                               <tr>
                                 <td colspan="16">
                                   <Button variant="default-outline" title="Collapse"
                                           onClick={()=>this.setState({expanded: false})} >
                                     <i><FontAwesomeIcon icon="arrow-up"/></i>
                                   </Button>
                                 </td>
                               </tr>
                               : <tr>
            <td colspan="16">
              <Button variant="default-outline"  title="Expand"
                      onClick={()=>this.setState({expanded: true})} >
                <i><FontAwesomeIcon icon="arrow-down"/></i>
              </Button>
            </td>
                                     </tr>) }
                  </tbody>
                </table>
              </div>
            </div>
        );
    }
};
